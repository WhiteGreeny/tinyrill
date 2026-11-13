// Package dial implements business connections through one customer's VPN.
// It deliberately has no underlay dialer or direct-connect fallback.
package dial

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/ipresource"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/stack"
)

var (
	ErrACLDenied          = errors.New("destination is not authorized by the VPN resources")
	ErrAddressFamily      = errors.New("VPN business connections require IPv4")
	ErrUnsupportedNetwork = errors.New("only TCP business connections are supported")
	ErrSessionUnavailable = errors.New("VPN session is unavailable")
)

type Dialer struct {
	stack         stack.Stack
	resolver      *resolve.Resolver
	ipResources   []client.IPResource
	resourceIndex *ipresource.Index
}

func (d *Dialer) DialIPPort(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if network != "tcp" && network != "tcp4" {
		return nil, ErrUnsupportedNetwork
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid destination: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return nil, ErrAddressFamily
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid destination port")
	}
	matched := false
	switch resources := ctx.Value(resolve.ContextKeyDomainResource).(type) {
	case []client.DomainResource:
		if resource, ok := matchDomainResourceForTunnel(resources, "tcp", port); ok {
			ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, resource)
			matched = true
		}
	case client.DomainResource:
		_, matched = matchDomainResourceForTunnel([]client.DomainResource{resources}, "tcp", port)
	}
	if !matched && d.resourceIndex != nil {
		if resource, ok := matchIPResourceForTunnel(d.resourceIndex, ip, "tcp", port); ok {
			ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, []client.DomainResource(nil))
			ctx = context.WithValue(ctx, resolve.ContextKeyIPResource, resource)
			matched = true
		}
	}
	// Missing resource data is not permission to send arbitrary destinations.
	if !matched {
		return nil, ErrACLDenied
	}
	if d.stack == nil {
		return nil, ErrSessionUnavailable
	}
	return d.stack.DialTCP(ctx, &net.TCPAddr{IP: ip.To4(), Port: port})
}

func matchDomainResourceForTunnel(resources []client.DomainResource, network string, port int) (client.DomainResource, bool) {
	if network == "tcp" {
		if resource, ok := client.MatchDomainResourceWhere(resources, network, port, func(resource client.DomainResource) bool {
			return !resource.EnableTCPPrefL3
		}); ok {
			return resource, true
		}
	}
	return client.MatchDomainResource(resources, network, port)
}

func matchesIPResource(index *ipresource.Index, target net.IP, network string, port int) bool {
	_, ok := matchIPResourceForTunnel(index, target, network, port)
	return ok
}

func matchIPResourceForTunnel(index *ipresource.Index, target net.IP, network string, port int) (client.IPResource, bool) {
	if network == "tcp" {
		if resource, ok := index.MatchWhere(target, network, port, func(resource client.IPResource) bool {
			return !resource.EnableTCPPrefL3
		}); ok {
			return resource, true
		}
	}
	return index.Match(target, network, port)
}

func (d *Dialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if network != "tcp" && network != "tcp4" {
		return nil, ErrUnsupportedNetwork
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid destination: %w", err)
	}
	if host == "" {
		return nil, errors.New("destination host is empty")
	}
	if net.ParseIP(host) == nil {
		if d.resolver == nil {
			return nil, resolve.ErrNoVPNDNS
		}
		var ip net.IP
		ctx, ip, err = d.resolver.Resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("VPN DNS: %w", err)
		}
		host = ip.String()
	}
	return d.DialIPPort(ctx, network, net.JoinHostPort(host, port))
}

func NewDialer(vpnStack stack.Stack, resolver *resolve.Resolver, resources []client.IPResource) *Dialer {
	return &Dialer{stack: vpnStack, resolver: resolver, ipResources: resources, resourceIndex: ipresource.New(resources)}
}
