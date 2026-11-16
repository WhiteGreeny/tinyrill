package resolve

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/ippool"
)

type failingVPNStack struct {
	mu        sync.Mutex
	addresses []string
}

func (*failingVPNStack) Run()                                                {}
func (*failingVPNStack) SetupIPPool(*ippool.IPPool[[]client.DomainResource]) {}
func (s *failingVPNStack) fail(address string) (net.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addresses = append(s.addresses, address)
	return nil, errors.New("VPN unavailable")
}
func (s *failingVPNStack) DialTCP(_ context.Context, a *net.TCPAddr) (net.Conn, error) {
	return s.fail(a.String())
}
func (s *failingVPNStack) DialUDP(_ context.Context, a *net.UDPAddr) (net.Conn, error) {
	return s.fail(a.String())
}

func TestEveryDNSAttemptUsesVPN(t *testing.T) {
	vpn := &failingVPNStack{}
	r := NewResolver(vpn, "10.0.0.53", "10.1.0.53", 60, nil, nil, true)
	defer r.Close()
	if _, _, err := r.Resolve(context.Background(), "business.internal"); err == nil {
		t.Fatal("failed VPN DNS succeeded")
	}
	vpn.mu.Lock()
	defer vpn.mu.Unlock()
	seen := map[string]bool{}
	for _, address := range vpn.addresses {
		if address != "10.0.0.53:53" && address != "10.1.0.53:53" {
			t.Fatalf("unexpected resolver %s", address)
		}
		seen[address] = true
	}
	if !seen["10.0.0.53:53"] || !seen["10.1.0.53:53"] {
		t.Fatalf("VPN DNS attempts: %v", seen)
	}
}

func TestMissingDNSDoesNotUseSystemResolver(t *testing.T) {
	r := NewResolver(&failingVPNStack{}, "", "", 60, nil, nil, true)
	defer r.Close()
	_, _, err := r.Resolve(context.Background(), "localhost")
	if !errors.Is(err, ErrNoVPNDNS) {
		t.Fatalf("missing VPN DNS must fail even for locally resolvable names: %v", err)
	}
	var lookup *LookupError
	if !errors.As(err, &lookup) || lookup.Host != "localhost" {
		t.Fatalf("missing DNS stage metadata: %v", err)
	}
}

func TestClosedResolverRejectsCachedEntries(t *testing.T) {
	r := NewResolver(nil, "", "", 60, nil, nil, false)
	r.SetPermanentDNS("internal.example", net.ParseIP("10.0.0.1"))
	r.Close()
	if _, _, err := r.Resolve(context.Background(), "internal.example"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed resolver: %v", err)
	}
}
