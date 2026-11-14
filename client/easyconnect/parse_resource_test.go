package easyconnect

import (
	"encoding/json"
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

func TestResourceDiagnosticObservesUnknownNameFieldWithoutChangingRules(t *testing.T) {
	var report resourcediag.Report
	c := NewClient(Options{Resources: ResourceOptions{IncludeDomains: true, Diagnostics: func(value resourcediag.Report) { report = value }}})
	defer c.Close()
	if err := c.parseResources(`<Resource><Rcs><Rc displayName="PRIVATE_NAME" type="1" proto="0" host="app.example" port="443~443"/></Rcs><Dns data=""/></Resource>`); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(report)
	if !strings.Contains(string(data), "/@displayName") || strings.Contains(string(data), "PRIVATE_NAME") || strings.Contains(string(data), "app.example") {
		t.Fatalf("bad diagnostic: %s", data)
	}
	if _, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 443); !ok {
		t.Fatal("diagnostic changed resource rules")
	}
}
