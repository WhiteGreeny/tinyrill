package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/things-go/go-socks5"
)

// Listeners reserves ports before authentication and owns all proxy streams.
// The listeners are only served after a VPN session has been established.
type Listeners struct {
	http, socks net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	pool        connectionPool
	server      *http.Server
	proxy       *httpProxy
	failures    chan error
	diagnostics failureLog
	once        sync.Once
	wg          sync.WaitGroup
}

func Reserve(httpAddr, socksAddr string) (*Listeners, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Listeners{ctx: ctx, cancel: cancel, failures: make(chan error, 2)}
	var err error
	if httpAddr != "" {
		s.http, err = net.Listen("tcp", httpAddr)
	}
	if err == nil && socksAddr != "" {
		s.socks, err = net.Listen("tcp", socksAddr)
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Listeners) Errors() <-chan error { return s.failures }

func (s *Listeners) Start(d *dial.Dialer, resolver *resolve.Resolver, timeout time.Duration) {
	if s.http != nil {
		s.proxy = newHTTPProxyWithDial(s.wrapDial(d.Dial, timeout))
		s.proxy.diagnostics = &s.diagnostics
		s.server = newHTTPServer(s.http.Addr().String(), s.proxy)
		s.server.BaseContext = func(net.Listener) context.Context { return s.ctx }
		s.serve(func() error { return s.server.Serve(&trackingListener{Listener: s.http, pool: &s.pool}) })
	}
	if s.socks != nil {
		server := socks5.NewServer(
			socks5.WithRule(&socks5.PermitCommand{EnableConnect: true}),
			socks5.WithResolver(&instanceResolver{ctx: s.ctx, resolver: resolver, diagnostics: &s.diagnostics}),
			socks5.WithDial(s.wrapDial(d.DialIPPort, timeout)),
			socks5.WithConnectMiddleware(func(_ context.Context, w io.Writer, _ *socks5.Request) error {
				if conn, ok := w.(net.Conn); ok {
					return conn.SetDeadline(time.Time{})
				}
				return nil
			}),
		)
		s.serve(func() error {
			return server.Serve(&trackingListener{Listener: s.socks, pool: &s.pool, handshakeTimeout: 30 * time.Second})
		})
	}
}

func (s *Listeners) serve(fn func() error) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := fn(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
			select {
			case s.failures <- err:
			default:
			}
		}
	}()
}

func (s *Listeners) wrapDial(fn client.DialContextFunc, timeout time.Duration) client.DialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		log.DebugPrintf("Proxy dial requested: network=%s target=%q", network, address)
		ctx, cancel := context.WithTimeout(ctx, timeout)
		stop := context.AfterFunc(s.ctx, cancel)
		defer func() { stop(); cancel() }()
		if err := s.ctx.Err(); err != nil {
			return nil, dial.ErrSessionUnavailable
		}
		conn, err := fn(ctx, network, address)
		if err != nil {
			if s.ctx.Err() == nil {
				s.diagnostics.report("dial", address, err)
			}
			return nil, &reportedFailure{err}
		}
		tracked, err := s.pool.add(conn)
		if err == nil {
			log.DebugPrintf("Proxy dial established: network=%s target=%q", network, address)
		}
		return tracked, err
	}
}

func (s *Listeners) Close() error {
	s.once.Do(func() {
		s.cancel()
		if s.http != nil {
			s.http.Close()
		}
		if s.socks != nil {
			s.socks.Close()
		}
		s.pool.close()
		if s.server != nil {
			s.server.Close()
		}
		if s.proxy != nil {
			s.proxy.close()
		}
		s.wg.Wait()
	})
	return nil
}

type instanceResolver struct {
	ctx         context.Context
	resolver    *resolve.Resolver
	diagnostics *failureLog
}

func (r *instanceResolver) Resolve(ctx context.Context, host string) (context.Context, net.IP, error) {
	queryCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	defer func() { stop(); cancel() }()
	resolved, ip, err := r.resolver.Resolve(queryCtx, host)
	if err != nil && r.ctx.Err() == nil && r.diagnostics != nil {
		r.diagnostics.report("dns", host, err)
	}
	// Preserve resource metadata without passing a cancelled lookup context to Dial.
	if value := resolved.Value(resolve.ContextKeyDomainResource); value != nil {
		ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, value)
	}
	if value := resolved.Value(resolve.ContextKeyResolveHost); value != nil {
		ctx = context.WithValue(ctx, resolve.ContextKeyResolveHost, value)
	}
	return ctx, ip, err
}

type connectionPool struct {
	mu          sync.Mutex
	closed      bool
	connections map[*trackedConn]struct{}
}
type trackedConn struct {
	net.Conn
	pool *connectionPool
	once sync.Once
}

func (p *connectionPool) add(conn net.Conn) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(p.connections) >= 2048 {
		conn.Close()
		return nil, net.ErrClosed
	}
	if p.connections == nil {
		p.connections = make(map[*trackedConn]struct{})
	}
	t := &trackedConn{Conn: conn, pool: p}
	p.connections[t] = struct{}{}
	return t, nil
}

func (c *trackedConn) Close() error {
	var err error
	c.once.Do(func() { err = c.Conn.Close(); c.pool.mu.Lock(); delete(c.pool.connections, c); c.pool.mu.Unlock() })
	return err
}

func (c *trackedConn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return nil
}

func (p *connectionPool) close() {
	p.mu.Lock()
	p.closed = true
	conns := make([]*trackedConn, 0, len(p.connections))
	for c := range p.connections {
		conns = append(conns, c)
	}
	p.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

type trackingListener struct {
	net.Listener
	pool             *connectionPool
	handshakeTimeout time.Duration
}

func (l *trackingListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.handshakeTimeout > 0 {
			c.SetDeadline(time.Now().Add(l.handshakeTimeout))
		}
		tracked, err := l.pool.add(c)
		if err == nil {
			return tracked, nil
		}
		l.pool.mu.Lock()
		closed := l.pool.closed
		l.pool.mu.Unlock()
		if closed {
			return nil, net.ErrClosed
		}
	}
}
