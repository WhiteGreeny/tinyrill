package instance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/client"
)

func TestResourceDisplaySeparatesAuthorizationFromReachability(t *testing.T) {
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: testProfile(t), Output: &out, Diagnostic: &diagnostic}
	ips := []client.IPResource{
		{IPMin: net.ParseIP("10.0.0.1"), IPMax: net.ParseIP("10.0.0.9"), PortMin: 1, PortMax: 65535, Protocol: "all"},
	}
	domains := client.DomainResources{
		"web.example":  {{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "internal-app-id"}, {PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "other-id"}},
		"dns.example":  {{PortMin: 53, PortMax: 53, Protocol: "udp"}},
		"ping.example": {{Protocol: "icmp"}},
	}
	f.Status(Status{State: "ready"})
	f.Resources(2, ips, domains)
	var status Status
	if err := json.Unmarshal(out.Bytes(), &status); err != nil || status.State != "ready" {
		t.Fatalf("resource display polluted JSON status: %q", out.String())
	}
	display := diagnostic.String()
	for _, want := range []string{"Customer a connected (generation 2)", "SOCKS5 listen:", "HTTP listen:", "4 distinct rules; 2 permit TCP", "reachability is not tested", "10.0.0.1 - 10.0.0.9", "1-65535"} {
		if !strings.Contains(display, want) {
			t.Errorf("missing %q in %s", want, display)
		}
	}
	for _, line := range strings.Split(display, "\n") {
		if strings.Contains(line, "dns.example") || strings.Contains(line, "ping.example") {
			if !strings.Contains(line, "unsupported") || strings.Contains(line, "permitted") {
				t.Errorf("non-TCP rule advertised as supported: %s", line)
			}
		}
	}
	if strings.Count(display, "web.example") != 1 || strings.Contains(display, "internal-app-id") || len(domains["web.example"]) != 2 {
		t.Fatal("presentation deduplication exposed metadata or changed routing rules")
	}
	diagnostic.Reset()
	f.Resources(2, ips, domains)
	if display != diagnostic.String() {
		t.Fatal("resource ordering is not stable")
	}
}

func TestResourceDisplayModesAndUntrustedLabels(t *testing.T) {
	var diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: testProfile(t), Diagnostic: &diagnostic}
	domains := make(client.DomainResources)
	for i := range 25 {
		domains[fmt.Sprintf("host%02d.example", i)] = []client.DomainResource{{Protocol: "tcp", PortMin: 80, PortMax: 80}}
	}
	f.Resources(1, nil, domains)
	if strings.Count(diagnostic.String(), ".example") != resourceSummaryLimit || !strings.Contains(diagnostic.String(), "Showing 20 of 25") {
		t.Fatal("summary must bound output and explain how to see the rest")
	}
	diagnostic.Reset()
	f.ResourceMode = "full"
	f.Resources(1, nil, domains)
	if strings.Count(diagnostic.String(), ".example") != 25 {
		t.Fatal("full display omitted rules")
	}
	diagnostic.Reset()
	f.ResourceMode = "none"
	f.Resources(1, nil, domains)
	if diagnostic.Len() != 0 {
		t.Fatal("none mode wrote a resource display")
	}
	f.ResourceMode = "summary"
	f.Resources(1, nil, nil)
	if !strings.Contains(diagnostic.String(), "No displayable authorization rules") {
		t.Fatal("empty resources should be explained")
	}
	diagnostic.Reset()
	f.Resources(1, nil, client.DomainResources{"bad\n\t\x1b[2J\u202e.example": {{Protocol: "udp", PortMin: 53, PortMax: 53}}})
	if strings.ContainsAny(diagnostic.String(), "\x1b\u202e\t") || strings.Contains(diagnostic.String(), "bad\n") {
		t.Fatal("gateway label injected terminal controls or a new row")
	}
	if len(resourceText(strings.Repeat("a", 10000))) > 259 {
		t.Fatal("unbounded display label")
	}
}
