package resolve

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/dnshosts"
)

func TestStaticHostsOverrideAnswersAndPreserveDomainAuthorization(t *testing.T) {
	hosts, err := dnshosts.New(map[string]string{"+.apps.internal": "10.0.0.10"})
	if err != nil {
		t.Fatal(err)
	}
	resources := client.DomainResources{"service.apps.internal": {{AppID: "authorized-app", Protocol: "tcp", PortMin: 443, PortMax: 443}}}
	r := NewResolver(nil, "", "", 60, resources, map[string][]net.IP{"service.apps.internal": {net.ParseIP("10.0.0.20")}, "gateway.internal": {net.ParseIP("10.0.0.30")}}, false, WithStaticHosts(hosts))
	defer r.Close()
	r.SetPermanentDNS("service.apps.internal", net.ParseIP("10.0.0.40"))
	ctx, ip, err := r.Resolve(context.Background(), "SERVICE.APPS.INTERNAL.")
	if err != nil || !ip.Equal(net.ParseIP("10.0.0.10")) {
		t.Fatalf("static override: %v %v", ip, err)
	}
	matched, ok := ctx.Value(ContextKeyDomainResource).([]client.DomainResource)
	if !ok || len(matched) != 1 || matched[0].AppID != "authorized-app" || matched[0].PortMin != 443 {
		t.Fatalf("authorization metadata lost: %#v", matched)
	}
	if _, ip, err := r.Resolve(context.Background(), "gateway.internal"); err != nil || !ip.Equal(net.ParseIP("10.0.0.30")) {
		t.Fatalf("gateway mapping changed: %v %v", ip, err)
	}
	if _, _, err := r.Resolve(context.Background(), "notapps.internal"); !errors.Is(err, ErrNoVPNDNS) {
		t.Fatalf("unmatched name did not fail closed: %v", err)
	}
	r.Close()
	if _, _, err := r.Resolve(context.Background(), "service.apps.internal"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed resolver served static mapping: %v", err)
	}
}
