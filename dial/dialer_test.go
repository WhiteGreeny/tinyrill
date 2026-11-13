package dial

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/ippool"
	"github.com/mythologyli/zju-connect/internal/ipresource"
	"github.com/mythologyli/zju-connect/internal/zcdns"
	"github.com/mythologyli/zju-connect/resolve"
)

func TestMatchesIPResourcePreservesRangePortAndProtocolRules(t *testing.T) {
	resources := []client.IPResource{{
		IPMin:    net.IPv4(10, 0, 0, 1),
		IPMax:    net.IPv4(10, 0, 0, 10),
		PortMin:  443,
		PortMax:  443,
		Protocol: "tcp",
	}}
	index := ipresource.New(resources)

	tests := []struct {
		name    string
		ip      net.IP
		network string
		port    int
		want    bool
	}{
		{name: "match", ip: net.IPv4(10, 0, 0, 5), network: "tcp", port: 443, want: true},
		{name: "outside range", ip: net.IPv4(10, 0, 0, 11), network: "tcp", port: 443},
		{name: "wrong port", ip: net.IPv4(10, 0, 0, 5), network: "tcp", port: 80},
		{name: "wrong protocol", ip: net.IPv4(10, 0, 0, 5), network: "udp", port: 443},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesIPResource(index, test.ip, test.network, test.port); got != test.want {
				t.Fatalf("matchesIPResource() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDialIPPortSelectsMatchingDomainResource(t *testing.T) {
	resources := []client.DomainResource{
		{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "https"},
		{PortMin: 80, PortMax: 80, Protocol: "tcp", AppID: "http"},
	}
	ctx := context.WithValue(context.Background(), resolve.ContextKeyDomainResource, resources)
	dialer := &Dialer{stack: &capturingStack{}, resourceIndex: ipresource.New(nil)}

	_, err := dialer.DialIPPort(ctx, "tcp", "121.194.4.13:443")
	if err != nil {
		t.Fatalf("DialIPPort() error = %v", err)
	}
	if got := dialer.stack.(*capturingStack).domainResource.AppID; got != "https" {
		t.Fatalf("selected AppID = %q, want https", got)
	}
}

func TestDialIPPortPrefersTCPTunnelResourceOverOverlappingL3Resource(t *testing.T) {
	resources := []client.DomainResource{
		{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "l3", EnableTCPPrefL3: true},
		{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "tcp"},
	}
	ctx := context.WithValue(context.Background(), resolve.ContextKeyDomainResource, resources)
	dialer := &Dialer{stack: &capturingStack{}, resourceIndex: ipresource.New(nil)}

	if _, err := dialer.DialIPPort(ctx, "tcp", "121.194.4.13:443"); err != nil {
		t.Fatalf("DialIPPort() error = %v", err)
	}
	if got := dialer.stack.(*capturingStack).domainResource.AppID; got != "tcp" {
		t.Fatalf("selected AppID = %q, want tcp", got)
	}
}

func TestDialIPPortCarriesTCPPrefL3IPResource(t *testing.T) {
	resource := client.IPResource{
		IPMin: net.IPv4(10, 0, 0, 42), IPMax: net.IPv4(10, 0, 0, 42), PortMin: 443, PortMax: 443,
		Protocol: "tcp", AppID: "l3", EnableTCPPrefL3: true,
	}
	stack := &capturingStack{}
	dialer := &Dialer{stack: stack, ipResources: []client.IPResource{resource}, resourceIndex: ipresource.New([]client.IPResource{resource})}

	if _, err := dialer.DialIPPort(context.Background(), "tcp", "10.0.0.42:443"); err != nil {
		t.Fatalf("DialIPPort() error = %v", err)
	}
	if !stack.ipResource.EnableTCPPrefL3 || stack.ipResource.AppID != "l3" {
		t.Fatalf("selected IP resource = %#v, want l3 TCP-prefers-L3", stack.ipResource)
	}
}

type capturingStack struct {
	calls          int
	domainResource client.DomainResource
	ipResource     client.IPResource
}

func TestIPResourceDoesNotCarryUnmatchedDomainAuthorization(t *testing.T) {
	s := &capturingStack{}
	resource := client.IPResource{IPMin: net.IPv4(10, 0, 0, 1), IPMax: net.IPv4(10, 0, 0, 1), PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "ip"}
	d := NewDialer(s, nil, []client.IPResource{resource})
	ctx := context.WithValue(context.Background(), resolve.ContextKeyDomainResource, client.DomainResource{Protocol: "tcp", PortMin: 22, PortMax: 22, AppID: "ssh-only"})
	if _, err := d.DialIPPort(ctx, "tcp", "10.0.0.1:443"); err != nil {
		t.Fatal(err)
	}
	if s.domainResource.AppID != "" || s.ipResource.AppID != "ip" {
		t.Fatal("unmatched domain authorization leaked into IP path")
	}
}

func (s *capturingStack) Run()                                                {}
func (s *capturingStack) SetupResolve(zcdns.LocalServer)                      {}
func (s *capturingStack) SetupIPPool(*ippool.IPPool[[]client.DomainResource]) {}
func (s *capturingStack) DialTCP(ctx context.Context, _ *net.TCPAddr) (net.Conn, error) {
	s.calls++
	if resource, ok := ctx.Value(resolve.ContextKeyDomainResource).(client.DomainResource); ok {
		s.domainResource = resource
	}
	if resource, ok := ctx.Value(resolve.ContextKeyIPResource).(client.IPResource); ok {
		s.ipResource = resource
	}
	return nil, nil
}
func (s *capturingStack) DialUDP(context.Context, *net.UDPAddr) (net.Conn, error) {
	return nil, nil
}

func TestBusinessDialFailsClosed(t *testing.T) {
	vpn := &capturingStack{}
	d := NewDialer(vpn, nil, []client.IPResource{{
		IPMin: net.ParseIP("10.0.0.1"), IPMax: net.ParseIP("10.0.0.1"),
		PortMin: 443, PortMax: 443, Protocol: "tcp",
	}})
	for _, test := range []struct {
		network, address string
		want             error
	}{
		{"tcp", "127.0.0.1:443", ErrACLDenied},
		{"tcp", "10.0.0.1:80", ErrACLDenied},
		{"tcp", "[::1]:443", ErrAddressFamily},
		{"tcp", "private.invalid:443", resolve.ErrNoVPNDNS},
		{"udp", "10.0.0.1:443", ErrUnsupportedNetwork},
		{"tcp", "10.0.0.1:0", nil},
		{"tcp", "10.0.0.1:65536", nil},
		{"tcp", "invalid", nil},
	} {
		t.Run(test.network+"/"+test.address, func(t *testing.T) {
			conn, err := d.Dial(context.Background(), test.network, test.address)
			if conn != nil || err == nil || (test.want != nil && !errors.Is(err, test.want)) {
				t.Fatalf("Dial = (%v, %v), want rejection %v", conn, err, test.want)
			}
		})
	}
	if vpn.calls != 0 {
		t.Fatalf("denied requests reached VPN: %d", vpn.calls)
	}
	if _, err := d.Dial(context.Background(), "tcp", "10.0.0.1:443"); err != nil {
		t.Fatal(err)
	}
	if vpn.calls != 1 {
		t.Fatal("authorized destination did not use VPN")
	}
}

func TestMissingResourcesNeverPermitDirectConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, resources := range [][]client.IPResource{nil, {}} {
		d := NewDialer(nil, nil, resources)
		if _, err := d.Dial(context.Background(), "tcp", listener.Addr().String()); !errors.Is(err, ErrACLDenied) {
			t.Fatalf("reachable local destination was not denied: %v", err)
		}
	}
}
