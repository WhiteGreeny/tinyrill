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
	for _, want := range []string{"Customer a connected (generation 2)", "SOCKS5 listen:", "HTTP listen:", "4 entries; 2 permit TCP", "reachability is not tested", "10.0.0.1 - 10.0.0.9", "1-65535"} {
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

func TestResourceNamesPreserveDistinctLabelsAndUnnamedTargets(t *testing.T) {
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: testProfile(t), Output: &out, Diagnostic: &diagnostic}
	domains := client.DomainResources{
		"shared.example": {
			{Name: "财务系统", Protocol: "tcp", PortMin: 443, PortMax: 443},
			{Name: "OA 系统", Protocol: "tcp", PortMin: 443, PortMax: 443},
			{Name: "财务系统", Protocol: "tcp", PortMin: 443, PortMax: 443},
		},
		"other.example":   {{Name: "财务系统", Protocol: "tcp", PortMin: 80, PortMax: 80}},
		"unnamed.example": {{Name: " \n ", Protocol: "tcp", PortMin: 22, PortMax: 22}},
	}
	ips := []client.IPResource{{Name: "内部服务", IPMin: net.ParseIP("10.0.0.1"), IPMax: net.ParseIP("10.0.0.9"), Protocol: "tcp", PortMin: 1, PortMax: 65535}}
	f.Resources(1, ips, domains)
	display := diagnostic.String()
	if !strings.Contains(display, "RESOURCE") || !strings.Contains(display, "5 entries; 5 permit TCP") ||
		strings.Count(display, "shared.example") != 2 || strings.Count(display, "财务系统") != 2 ||
		!strings.Contains(display, "OA 系统") || !strings.Contains(display, "内部服务") {
		t.Fatalf("names or distinct targets lost: %s", display)
	}
	for _, line := range strings.Split(display, "\n") {
		if strings.Contains(line, "unnamed.example") && !strings.HasSuffix(line, "-") {
			t.Fatalf("missing-name fallback: %s", line)
		}
	}
	if len(domains["shared.example"]) != 3 || out.Len() != 0 {
		t.Fatal("name display modified routing data or stdout")
	}
	diagnostic.Reset()
	f.Resources(1, ips, domains)
	if display != diagnostic.String() {
		t.Fatal("named rows are not stably sorted")
	}
}

func TestResourceDetailsKeepDescriptionsGroupsAndNotes(t *testing.T) {
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: testProfile(t), Output: &out, Diagnostic: &diagnostic, ResourceMode: "full"}
	base := client.DomainResource{Name: "应用名称", Description: "应用描述", GroupName: "分组名称", GroupDescription: "分组描述", Protocol: "tcp", PortMin: 443, PortMax: 443}
	rules := []client.DomainResource{base, base}
	for _, change := range []func(*client.DomainResource){
		func(r *client.DomainResource) { r.Description = "另一应用描述" },
		func(r *client.DomainResource) { r.GroupName = "另一分组名称" },
		func(r *client.DomainResource) { r.GroupDescription = "另一分组描述" },
		func(r *client.DomainResource) { r.Note = "另一备注" },
	} {
		rule := base
		change(&rule)
		rules = append(rules, rule)
	}
	domains := client.DomainResources{
		"shared.example":           rules,
		"description-only.example": {{Name: " ", Description: "只有应用描述", GroupName: "", GroupDescription: "只有分组描述", Protocol: "tcp", PortMin: 80, PortMax: 80}},
		"same-label.example":       {{Name: "相同文字", Description: "相同文字", Protocol: "tcp", PortMin: 443, PortMax: 443}},
	}
	ips := []client.IPResource{
		{Name: "EC 资源", Note: "EC 备注", IPMin: net.ParseIP("10.0.0.1"), IPMax: net.ParseIP("10.0.0.1"), Protocol: "tcp", PortMin: 22, PortMax: 22},
		{Description: "IP 应用描述", GroupName: "IP 分组", GroupDescription: "IP 分组说明", IPMin: net.ParseIP("10.0.1.1"), IPMax: net.ParseIP("10.0.1.9"), Protocol: "tcp", PortMin: 80, PortMax: 81},
	}
	f.Resources(1, ips, domains)
	display := diagnostic.String()
	for _, want := range []string{"9 entries; 9 permit TCP", "Name: 应用名称", "Description: 应用描述", "Group name: 分组名称", "Group description: 分组描述", "Description: 另一应用描述", "Group name: 另一分组名称", "Group description: 另一分组描述", "Note: 另一备注", "Description: 只有应用描述", "Group description: 只有分组描述", "Name: 相同文字", "Description: 相同文字", "Name: EC 资源", "Note: EC 备注", "Description: IP 应用描述", "Group name: IP 分组", "Group description: IP 分组说明"} {
		if !strings.Contains(display, want) {
			t.Errorf("missing %q in %s", want, display)
		}
	}
	if strings.Count(display, "shared.example") != 5 || len(domains["shared.example"]) != 6 || out.Len() != 0 {
		t.Fatal("metadata-aware display deduplication changed rules or lost distinct labels")
	}
	for _, line := range strings.Split(display, "\n") {
		if strings.Contains(line, "description-only.example") && !strings.Contains(line, "Description: 只有应用描述") {
			t.Fatal("empty name did not leave the description visible on the resource row")
		}
	}
	diagnostic.Reset()
	f.Resources(1, ips, domains)
	if display != diagnostic.String() {
		t.Fatal("metadata ordering is not stable")
	}
	t.Log(display)
}

func TestResourceNamesCannotInjectTerminalRowsOrUnboundedText(t *testing.T) {
	var diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: testProfile(t), Diagnostic: &diagnostic}
	f.Resources(1, nil, client.DomainResources{
		"safe.example": {{Name: "财务\nFAKE_ROW\t\x1b[2J\u202e", Description: "说明\nFAKE_ROW\t\x1b[2J\u202e", GroupName: "分组\nFAKE_ROW\t\x1b[2J\u202e", GroupDescription: "分组说明\nFAKE_ROW\t\x1b[2J\u202e", Note: "备注\nFAKE_ROW\t\x1b[2J\u202e", Protocol: "tcp", PortMin: 443, PortMax: 443}},
		"long.example": {{Name: strings.Repeat("名", 1000), Protocol: "tcp", PortMin: 80, PortMax: 80}},
	})
	display := diagnostic.String()
	if strings.ContainsAny(display, "\x1b\u202e\t") || strings.Contains(display, "\nFAKE_ROW") || strings.Count(display, "名") != 256 {
		t.Fatalf("unsafe resource name display: %s", display)
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
