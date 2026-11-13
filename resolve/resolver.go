package resolve

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/ippool"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/stack"
	"github.com/patrickmn/go-cache"
)

type Resolver struct {
	remoteUDPResolver *net.Resolver
	remoteTCPResolver *net.Resolver
	secondaryResolver *net.Resolver
	ttl               uint64
	domainIndex       *domainResourceIndex
	dnsResource       map[string][]net.IP
	dnsResourceCursor sync.Map
	useRemoteDNS      bool

	dnsCache *cache.Cache

	IPPool *ippool.IPPool[[]client.DomainResource]

	timer  *time.Timer
	useTCP bool
	// check to use tcp resolver or udp resolver
	tcpLock           sync.RWMutex
	resolutionMu      sync.Mutex
	resolutions       map[string]*sharedResolution
	resolutionClosed  bool
	activeResolutions atomic.Int64

	closeOnce sync.Once
}

const remoteDNSTCPFallbackDelay = 300 * time.Millisecond

var ErrNoVPNDNS = errors.New("no VPN DNS server or resource mapping is available")

type lookupIPFunc func(context.Context, string, string) ([]net.IP, error)

type dnsLookupResult struct {
	ips []net.IP
	err error
}

type sharedResolution struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	ip      net.IP
	err     error
}

func (r *Resolver) coordinationEntryCount() int {
	return int(r.activeResolutions.Load())
}

type contextKey string

var (
	ContextKeyFakeIP          = contextKey("FAKE_IP")
	ContextKeyResolveHost     = contextKey("RESOLVE_HOST")
	ContextKeyDomainResource  = contextKey("DOMAIN_RESOURCE")
	ContextKeyIPResource      = contextKey("IP_RESOURCE")
	contextKeyIgnoreTCPPrefL3 = contextKey("IGNORE_TCP_PREF_L3")
)

func WithIgnoreTCPPrefL3(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyIgnoreTCPPrefL3, true)
}

func IgnoreTCPPrefL3(ctx context.Context) bool {
	ignore, _ := ctx.Value(contextKeyIgnoreTCPPrefL3).(bool)
	return ignore
}

func TCPPrefersL3(ctx context.Context) bool {
	if resource, ok := ctx.Value(ContextKeyDomainResource).(client.DomainResource); ok {
		return resource.EnableTCPPrefL3
	}
	if resource, ok := ctx.Value(ContextKeyIPResource).(client.IPResource); ok {
		return resource.EnableTCPPrefL3
	}
	return false
}

// Resolve ip address. If the host could be visited via VPN, this function set a DOMAIN_RESOURCE value in context. If resolve success, this function set a RESOLVE_HOST value in context.
func (r *Resolver) Resolve(ctx context.Context, host string) (resCtx context.Context, resIP net.IP, resErr error) {
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	r.resolutionMu.Lock()
	closed := r.resolutionClosed
	r.resolutionMu.Unlock()
	if closed {
		return ctx, nil, net.ErrClosed
	}
	host = normalizeHostname(host)
	defer func() {
		if resErr == nil {
			resCtx = context.WithValue(resCtx, ContextKeyResolveHost, host)
		}
	}()
	var domainResourceFound = false
	var domainResources []client.DomainResource
	if domain, resources, found := matchDomainResource(r.domainIndex, host); found {
		domainResourceFound = true
		domainResources = resources
		ctx = context.WithValue(ctx, ContextKeyDomainResource, resources)
		log.DebugPrintf("Domain resource found: %s", domain)
	}

	if cachedIP, found := r.getDNSCache(host); found {
		log.Printf("%s -> %s", host, cachedIP.String())
		return ctx, cachedIP, nil
	}

	if r.dnsResource != nil {
		if ips, found := r.dnsResource[host]; found && len(ips) > 0 {
			cursorValue, _ := r.dnsResourceCursor.LoadOrStore(host, &atomic.Uint64{})
			cursor := cursorValue.(*atomic.Uint64)
			ip := ips[(cursor.Add(1)-1)%uint64(len(ips))]
			log.Printf("%s -> %s", host, ip.String())
			if domainResourceFound {
				err := r.IPPool.SetIPDomain(ip, host, domainResources)
				if err != nil {
					log.DebugPrintf("Set IP err: %s", err)
				}
			}
			return ctx, ip, nil
		}

		if fakeIPValue := ctx.Value(ContextKeyFakeIP); fakeIPValue != nil {
			if domainResourceFound {
				ip := r.IPPool.GenerateIP(host, domainResources)
				log.Printf("%s -> %s (Fake IP)", host, ip.String())
				return ctx, ip, nil
			}
		}
	}

	if r.useRemoteDNS && r.remoteUDPResolver != nil && r.remoteTCPResolver != nil {
		ip, err := r.resolveCoordinated(ctx, host, func(lookupCtx context.Context) (net.IP, error) {
			return r.resolveRemote(lookupCtx, host)
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx, nil, ctx.Err()
			}
			if r.secondaryResolver == nil {
				return ctx, nil, err
			}
			return r.ResolveWithSecondaryDNS(ctx, host)
		}
		log.Printf("%s -> %s", host, ip.String())
		return ctx, ip, nil
	} else {
		return r.ResolveWithSecondaryDNS(ctx, host)
	}
}

func (r *Resolver) resolveCoordinated(ctx context.Context, host string, lookup func(context.Context) (net.IP, error)) (net.IP, error) {
	r.resolutionMu.Lock()
	if r.resolutionClosed {
		r.resolutionMu.Unlock()
		return nil, net.ErrClosed
	}
	call := r.resolutions[host]
	if call == nil {
		lookupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		call = &sharedResolution{ctx: lookupCtx, cancel: cancel, done: make(chan struct{})}
		if r.resolutions == nil {
			r.resolutions = make(map[string]*sharedResolution)
		}
		r.resolutions[host] = call
		r.activeResolutions.Add(1)
		go r.runResolution(host, call, lookup)
	}
	call.waiters++
	r.resolutionMu.Unlock()

	select {
	case <-ctx.Done():
		r.releaseResolutionWaiter(host, call)
		return nil, ctx.Err()
	case <-call.done:
		return call.ip, call.err
	}
}

func (r *Resolver) runResolution(host string, call *sharedResolution, lookup func(context.Context) (net.IP, error)) {
	call.ip, call.err = lookup(call.ctx)
	r.resolutionMu.Lock()
	if r.resolutions[host] == call {
		delete(r.resolutions, host)
	}
	r.resolutionMu.Unlock()
	call.cancel()
	r.activeResolutions.Add(-1)
	close(call.done)
}

func (r *Resolver) releaseResolutionWaiter(host string, call *sharedResolution) {
	r.resolutionMu.Lock()
	if r.resolutions[host] == call {
		call.waiters--
		if call.waiters == 0 {
			delete(r.resolutions, host)
			call.cancel()
		}
	}
	r.resolutionMu.Unlock()
}

func matchDomainResource(index *domainResourceIndex, host string) (string, []client.DomainResource, bool) {
	return index.Match(host)
}

func (r *Resolver) resolveRemote(ctx context.Context, host string) (net.IP, error) {
	r.tcpLock.RLock()
	useTCP := r.useTCP
	r.tcpLock.RUnlock()

	if useTCP {
		ips, err := r.remoteTCPResolver.LookupIP(ctx, "ip4", host)
		return r.cacheFirstIP(host, ips, err)
	}

	ips, udpFailed, err := lookupIPWithTCPFallback(
		ctx,
		host,
		r.remoteUDPResolver.LookupIP,
		r.remoteTCPResolver.LookupIP,
		remoteDNSTCPFallbackDelay,
	)
	if err != nil {
		return nil, err
	}
	if udpFailed {
		r.preferTCPTemporarily()
	}
	return r.cacheFirstIP(host, ips, nil)
}

func lookupIPWithTCPFallback(ctx context.Context, host string, udpLookup, tcpLookup lookupIPFunc, fallbackDelay time.Duration) ([]net.IP, bool, error) {
	lookupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	udpResult := make(chan dnsLookupResult, 1)
	go func() {
		ips, err := udpLookup(lookupCtx, "ip4", host)
		udpResult <- dnsLookupResult{ips: ips, err: err}
	}()

	timer := time.NewTimer(fallbackDelay)
	defer timer.Stop()
	var tcpResult chan dnsLookupResult
	var udpErr, tcpErr error
	udpDone := false
	tcpDone := false

	startTCP := func() {
		if tcpResult != nil {
			return
		}
		tcpResult = make(chan dnsLookupResult, 1)
		go func() {
			ips, err := tcpLookup(lookupCtx, "ip4", host)
			tcpResult <- dnsLookupResult{ips: ips, err: err}
		}()
	}

	for {
		select {
		case result := <-udpResult:
			udpDone = true
			if result.err == nil {
				return result.ips, false, nil
			}
			udpErr = result.err
			startTCP()
			if tcpDone {
				return nil, true, errors.Join(udpErr, tcpErr)
			}
		case result := <-tcpResult:
			tcpDone = true
			if result.err == nil {
				return result.ips, udpDone, nil
			}
			tcpErr = result.err
			if udpDone {
				return nil, true, errors.Join(udpErr, tcpErr)
			}
		case <-timer.C:
			startTCP()
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (r *Resolver) preferTCPTemporarily() {
	r.tcpLock.Lock()
	defer r.tcpLock.Unlock()
	r.useTCP = true
	if r.timer == nil {
		r.timer = time.AfterFunc(10*time.Minute, func() {
			r.tcpLock.Lock()
			r.useTCP = false
			r.timer = nil
			r.tcpLock.Unlock()
		})
	}
}

func (r *Resolver) cacheFirstIP(host string, ips []net.IP, err error) (net.IP, error) {
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("DNS lookup returned no addresses")
	}
	r.setDNSCache(host, ips[0])
	return ips[0], nil
}

func normalizeHostname(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func (r *Resolver) RemoteUDPResolver() (*net.Resolver, error) {
	if r.remoteUDPResolver != nil {
		return r.remoteUDPResolver, nil
	} else {
		return nil, errors.New("remote UDP resolver is nil")
	}
}

func (r *Resolver) RemoteTCPResolver() (*net.Resolver, error) {
	if r.remoteTCPResolver != nil {
		return r.remoteTCPResolver, nil
	} else {
		return nil, errors.New("remote TCP resolver is nil")
	}
}

// ResolveWithSecondaryDNS uses only another resolver reachable through the VPN.
func (r *Resolver) ResolveWithSecondaryDNS(ctx context.Context, host string) (context.Context, net.IP, error) {
	if r.secondaryResolver == nil {
		return ctx, nil, ErrNoVPNDNS
	}
	ip, err := r.resolveCoordinated(ctx, "secondary:"+host, func(lookupCtx context.Context) (net.IP, error) {
		ips, err := r.secondaryResolver.LookupIP(lookupCtx, "ip4", host)
		return r.cacheFirstIP(host, ips, err)
	})
	return ctx, ip, err
}
func (r *Resolver) Close() {
	r.closeOnce.Do(func() {
		r.resolutionMu.Lock()
		r.resolutionClosed = true
		for host, call := range r.resolutions {
			delete(r.resolutions, host)
			call.cancel()
		}
		r.resolutionMu.Unlock()
		r.tcpLock.Lock()
		if r.timer != nil {
			r.timer.Stop()
			r.timer = nil
		}
		r.tcpLock.Unlock()
	})
}

// newVPNResolver cannot fall back to net.DefaultResolver or the host network.
func newVPNResolver(vpnStack stack.Stack, server string, forceTCP bool) *net.Resolver {
	ip := net.ParseIP(server)
	if vpnStack == nil || ip == nil || ip.To4() == nil {
		return nil
	}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		if forceTCP || network == "tcp" {
			return vpnStack.DialTCP(ctx, &net.TCPAddr{IP: ip.To4(), Port: 53})
		}
		return vpnStack.DialUDP(ctx, &net.UDPAddr{IP: ip.To4(), Port: 53})
	}}
}

func NewResolver(vpnStack stack.Stack, remoteDNSServer, secondaryDNSServer string, ttl uint64, domainResources client.DomainResources, dnsResource map[string][]net.IP, useRemoteDNS bool) *Resolver {
	if ttl == 0 {
		ttl = 300
	}
	resolver := &Resolver{
		remoteUDPResolver: newVPNResolver(vpnStack, remoteDNSServer, false),
		remoteTCPResolver: newVPNResolver(vpnStack, remoteDNSServer, true),
		secondaryResolver: newVPNResolver(vpnStack, secondaryDNSServer, true),
		ttl:               ttl,
		domainIndex:       newDomainResourceIndex(domainResources),
		dnsResource:       dnsResource,
		dnsCache:          cache.New(time.Duration(ttl)*time.Second, time.Duration(ttl)*2*time.Second),
		useRemoteDNS:      useRemoteDNS,
	}
	// This constant IPv4 pool is protocol-internal; it does not create a host TUN.
	resolver.IPPool, _ = ippool.NewIPPool[[]client.DomainResource]("198.18.0.0/16")
	return resolver
}
