package easyconnect

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/resourcediag"
)

func TestParseResourcesPreservesMultipleRulesForDomain(t *testing.T) {
	resource := `<Resource><Rcs>
		<Rc type="1" proto="0" host="www.cnki.net" port="443~443" />
		<Rc type="1" proto="0" host="www.cnki.net" port="80~80" />
	</Rcs><Dns data="" dnsserver="10.0.0.1" /></Resource>`
	c := &Client{useDomainResource: true}
	if err := c.parseResources(resource); err != nil {
		t.Fatalf("parseResources() error = %v", err)
	}

	rules := c.domainResources["www.cnki.net"]
	if len(rules) != 2 {
		t.Fatalf("domain rules = %#v, want 2 rules", rules)
	}
	for _, port := range []int{80, 443} {
		if _, ok := client.MatchDomainResource(rules, "tcp", port); !ok {
			t.Fatalf("domain rules did not match tcp/%d: %#v", port, rules)
		}
	}
}

func TestResourceDiagnosticStillOmitsPreservedNameValues(t *testing.T) {
	var report resourcediag.Report
	c := NewClient(Options{Resources: ResourceOptions{IncludeDomains: true, Diagnostics: func(value resourcediag.Report) { report = value }}})
	defer c.Close()
	if err := c.parseResources(`<Resource><Rcs><Rc name="PRIVATE_NAME" type="1" proto="0" host="app.example" port="443~443"/></Rcs><Dns data=""/></Resource>`); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(report)
	if !strings.Contains(string(data), "/@name") || strings.Contains(string(data), "PRIVATE_NAME") || strings.Contains(string(data), "app.example") {
		t.Fatalf("bad diagnostic: %s", data)
	}
	if rule, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 443); !ok || rule.Name != "PRIVATE_NAME" {
		t.Fatal("diagnostic changed resource rules")
	}
}

func TestECResourceNameFollowsEveryAddressAndKeepsAuthorizationRules(t *testing.T) {
	c := &Client{useDomainResource: true}
	if err := c.parseResources(`<Resource><Rcs>
		<Rc name="财务 &amp; OA 100%+服务" type="1" proto="0" host="app.example;10.1.0.1;10.2.0.1~10.2.0.9" port="443~443;8443~8443;80~81"/>
		<Rc name="另一系统" type="2" proto="0" host="app.example" port="443~443"/>
		<Rc type="1" proto="0" host="unnamed.example" port="22~22"/>
	</Rcs></Resource>`); err != nil {
		t.Fatal(err)
	}
	const wantName = "财务 & OA 100%+服务"
	if got := c.domainResources["app.example"]; len(got) != 2 || got[0].Name != wantName || got[1].Name != "另一系统" {
		t.Fatalf("domain names = %#v", got)
	}
	if len(c.ipResources) != 2 || c.ipResources[0].Name != wantName || c.ipResources[1].Name != wantName {
		t.Fatalf("IP names = %#v", c.ipResources)
	}
	if c.ipResources[0].PortMin != 8443 || !c.ipResources[1].IPMin.Equal(net.ParseIP("10.2.0.1")) || !c.ipResources[1].IPMax.Equal(net.ParseIP("10.2.0.9")) || c.ipResources[1].PortMin != 80 || c.ipResources[1].PortMax != 81 {
		t.Fatal("name parsing changed IP/port ranges")
	}
	if rule, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 443); !ok || rule.Name != wantName {
		t.Fatal("named resource no longer matches its authorized target")
	}
	if _, ok := client.MatchDomainResource(c.domainResources["app.example"], "udp", 443); ok {
		t.Fatal("name changed protocol authorization")
	}
	if _, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 80); ok {
		t.Fatal("name changed port authorization")
	}
	if got := c.domainResources["unnamed.example"]; len(got) != 1 || got[0].Name != "" {
		t.Fatal("missing name should preserve the address rule")
	}
}

func TestResourceDiagnosticTruncationPreservesAllNamedResources(t *testing.T) {
	var report resourcediag.Report
	c := NewClient(Options{Resources: ResourceOptions{IncludeDomains: true, Diagnostics: func(value resourcediag.Report) { report = value }}})
	defer c.Close()
	var xml strings.Builder
	xml.WriteString(`<Resource><Rcs>`)
	for i := range 12 {
		fmt.Fprintf(&xml, `<Rc name="系统%d" type="1" proto="0" host="app%d.example" port="443~443"/>`, i, i)
	}
	xml.WriteString(`</Rcs></Resource>`)
	if err := c.parseResources(xml.String()); err != nil {
		t.Fatal(err)
	}
	if !report.Truncated || len(c.domainResources) != 12 {
		t.Fatalf("diagnostic truncated=%v, parsed resource count=%d", report.Truncated, len(c.domainResources))
	}
	if got := c.domainResources["app11.example"]; len(got) != 1 || got[0].Name != "系统11" {
		t.Fatal("resource beyond diagnostic sample lost its name")
	}
}
