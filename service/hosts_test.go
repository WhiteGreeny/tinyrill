package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/internal/dnshosts"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/stack"
	socksproxy "golang.org/x/net/proxy"
)

type staticHostsVPN struct {
	stack.Stack
	ip    string
	calls atomic.Int32
	hosts chan string
}

type staticHostsConn struct{ net.Conn }

func (c *staticHostsConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(10, 255, 0, 2), Port: 12345}
}

func (s *staticHostsVPN) DialTCP(_ context.Context, addr *net.TCPAddr) (net.Conn, error) {
	s.calls.Add(1)
	if addr.String() != net.JoinHostPort(s.ip, "80") {
		return nil, fmt.Errorf("unexpected VPN destination: %s", addr)
	}
	conn, peer := net.Pipe()
	go func() {
		defer peer.Close()
		peer.SetDeadline(time.Now().Add(3 * time.Second))
		req, err := http.ReadRequest(bufio.NewReader(peer))
		if err != nil {
			return
		}
		defer req.Body.Close()
		s.hosts <- req.Host
		fmt.Fprint(peer, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	}()
	return &staticHostsConn{conn}, nil
}
func (s *staticHostsVPN) DialUDP(context.Context, *net.UDPAddr) (net.Conn, error) {
	return nil, errors.New("unexpected DNS/UDP attempt")
}

func TestStaticHostsWorkThroughBothProxiesWithoutGrantingAccess(t *testing.T) {
	for _, proxyType := range []string{"http", "socks5"} {
		t.Run(proxyType, func(t *testing.T) {
			type customer struct {
				client *http.Client
				vpn    *staticHostsVPN
			}
			var customers []customer
			for _, customerIP := range []string{"10.0.0.10", "10.1.0.10"} {
				hosts, err := dnshosts.New(map[string]string{"+.apps.internal": customerIP, "blocked.internal": "10.9.0.10"})
				if err != nil {
					t.Fatal(err)
				}
				vpn := &staticHostsVPN{ip: customerIP, hosts: make(chan string, 2)}
				r := resolve.NewResolver(vpn, "", "", 60, nil, nil, false, resolve.WithStaticHosts(hosts))
				defer r.Close()
				ip := net.ParseIP(customerIP)
				d := dial.NewDialer(vpn, r, []client.IPResource{{IPMin: ip, IPMax: ip, PortMin: 80, PortMax: 80, Protocol: "tcp"}})
				s, err := Reserve("127.0.0.1:0", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				s.Start(d, r, time.Second)
				transport := &http.Transport{DisableKeepAlives: true}
				defer transport.CloseIdleConnections()
				if proxyType == "http" {
					endpoint, _ := url.Parse("http://" + s.http.Addr().String())
					transport.Proxy = http.ProxyURL(endpoint)
				} else {
					proxyDialer, err := socksproxy.SOCKS5("tcp", s.socks.Addr().String(), nil, &net.Dialer{Timeout: time.Second})
					if err != nil {
						t.Fatal(err)
					}
					transport.DialContext = proxyDialer.(socksproxy.ContextDialer).DialContext
				}
				httpClient := &http.Client{Transport: transport, Timeout: 3 * time.Second}
				customers = append(customers, customer{httpClient, vpn})
			}
			// Both instances are alive before either request. The same hostname
			// must resolve to each customer's own destination.
			for _, c := range customers {
				httpClient, vpn := c.client, c.vpn
				response, err := httpClient.Get("http://nested.service.apps.internal/")
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != 200 || string(body) != "ok" {
					t.Fatalf("mapped request: %d %q %v", response.StatusCode, body, err)
				}
				if host := <-vpn.hosts; host != "nested.service.apps.internal" {
					t.Fatalf("HTTP Host rewritten: %q", host)
				}
				response, err = httpClient.Get("http://blocked.internal/")
				if response != nil {
					response.Body.Close()
				}
				if proxyType == "http" && (err != nil || response.StatusCode != 500) {
					t.Fatalf("unauthorized HTTP mapping: %v %v", response, err)
				}
				if proxyType == "socks5" && err == nil {
					t.Fatal("SOCKS mapping granted unauthorized access")
				}
				if vpn.calls.Load() != 1 {
					t.Fatalf("unexpected VPN connection attempts: %d", vpn.calls.Load())
				}
			}
		})
	}
}
