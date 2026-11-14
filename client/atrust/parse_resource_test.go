package atrust

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/resourcediag"
)

func TestParseResourcePreservesMultipleRulesForDomain(t *testing.T) {
	var policy ClientResource
	appInfo := resourceGroup{}
	for _, rule := range []struct{ id, group, port string }{
		{id: "https-app", group: "https-group", port: "443"},
		{id: "http-app", group: "http-group", port: "80"},
	} {
		var app resourceApp
		app.ID, app.NodeGroupID, app.AccessModel = rule.id, rule.group, "L3VPN"
		app.AddressList = append(app.AddressList, resourceAddress{Protocol: "tcp", Port: rule.port, Host: "www.cnki.net"})
		appInfo.Apps = append(appInfo.Apps, app)
	}
	policy.Data.AppList.Data.AppInfo = append(policy.Data.AppList.Data.AppInfo, appInfo)
	resource, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	c := &Client{}
	if err := c.parseResource(resource); err != nil {
		t.Fatalf("parseResource() error = %v", err)
	}

	rules := c.domainResources["www.cnki.net"]
	if len(rules) != 2 {
		t.Fatalf("domain rules = %#v, want 2 rules", rules)
	}
	if got, ok := client.MatchDomainResource(rules, "tcp", 443); !ok || got.AppID != "https-app" || got.NodeGroupID != "https-group" {
		t.Fatalf("443 rule = (%#v, %v), want https app metadata", got, ok)
	}
	if got, ok := client.MatchDomainResource(rules, "tcp", 80); !ok || got.AppID != "http-app" || got.NodeGroupID != "http-group" {
		t.Fatalf("80 rule = (%#v, %v), want http app metadata", got, ok)
	}
}

func TestATrustResourceMetadataFollowsAllAddressForms(t *testing.T) {
	resource := []byte(`{"data":{"appList":{"data":{"appInfo":[
		{"name":"分组 A","description":"分组说明 A","apps":[
			{"name":"应用 A","description":"  应用说明 & 100%+  ","id":"app-a","nodeGroupId":"routing-group","accessModel":"L3VPN","enableTCPPrefL3":true,"addrPretend":false,"addressList":[
				{"protocol":"tcp","port":"443","host":"app.example","ip":["10.0.0.2"]},
				{"protocol":"tcp","port":"8443","host":"10.1.0.2"},
				{"protocol":"tcp","port":"80-81","host":"10.2.0.1-10.2.0.9"},
				{"protocol":"tcp","port":"22","host":"10.3.0.0/24"}]},
			{"name":"","description":"只有应用描述","id":"unnamed","accessModel":"L3VPN","addressList":[{"protocol":"tcp","port":"443","host":"unnamed.example"}]}]},
		{"name":"","description":"只有分组描述","apps":[
			{"name":"","description":"另一个分组中的应用","id":"app-b","accessModel":"L3VPN","addressList":[{"protocol":"tcp","port":"443","host":"app.example"}]}]}
	]}}}}`)
	c := &Client{}
	if err := c.parseResource(resource); err != nil {
		t.Fatal(err)
	}
	rules := c.domainResources["app.example"]
	if len(rules) != 2 || rules[0].Name != "应用 A" || rules[0].Description != "  应用说明 & 100%+  " || rules[0].GroupName != "分组 A" || rules[0].GroupDescription != "分组说明 A" {
		t.Fatalf("domain labels lost: %#v", rules)
	}
	if rules[0].AppID != "app-a" || rules[0].NodeGroupID != "routing-group" || !rules[0].EnableTCPPrefL3 || rules[0].AddrPretend {
		t.Fatal("display metadata changed tunnel policy")
	}
	if rules[1].Name != "" || rules[1].Description != "另一个分组中的应用" || rules[1].GroupName != "" || rules[1].GroupDescription != "只有分组描述" || rules[1].AppID != "app-b" {
		t.Fatal("descriptions were discarded or copied into empty names")
	}
	if got := c.domainResources["unnamed.example"]; len(got) != 1 || got[0].Name != "" || got[0].Description != "只有应用描述" || got[0].GroupName != "分组 A" {
		t.Fatal("empty app name lost its description or parent group")
	}
	if len(c.ipResources) != 4 {
		t.Fatalf("IP rules = %d, want DNS-derived, single, range and CIDR", len(c.ipResources))
	}
	for _, ip := range c.ipResources {
		if ip.Name != rules[0].Name || ip.Description != rules[0].Description || ip.GroupName != rules[0].GroupName || ip.GroupDescription != rules[0].GroupDescription || ip.AppID != "app-a" || ip.NodeGroupID != "routing-group" || !ip.EnableTCPPrefL3 {
			t.Fatalf("IP labels or routing metadata lost: %#v", ip)
		}
	}
	if rule, ok := c.resourceIndex.Match(net.ParseIP("10.3.0.12"), "tcp", 22); !ok || rule.Name != "应用 A" || rule.GroupDescription != "分组说明 A" {
		t.Fatal("CIDR index lost metadata or authorization")
	}
	if _, ok := client.MatchDomainResource(rules, "udp", 443); ok {
		t.Fatal("labels expanded protocol authorization")
	}
	if _, ok := client.MatchDomainResource(rules, "tcp", 80); ok {
		t.Fatal("labels expanded port authorization")
	}
}

func TestATrustNonTextLabelsDoNotRejectAuthorization(t *testing.T) {
	c := &Client{}
	if err := c.parseResource([]byte(`{"data":{"appList":{"data":{"appInfo":[{"name":12,"description":null,"apps":[{"name":{},"description":false,"accessModel":"L3VPN","addressList":[{"protocol":"tcp","port":"443","host":"app.example"}]}]}]}}}}`)); err != nil {
		t.Fatal(err)
	}
	rule, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 443)
	if !ok || rule.Name != "" || rule.Description != "" || rule.GroupName != "" || rule.GroupDescription != "" {
		t.Fatalf("unexpected display label types affected authorization: %#v", rule)
	}
}

func TestResourceDiagnosticObservesUnknownNameFieldWithoutChangingRules(t *testing.T) {
	var report resourcediag.Report
	var snapshot resourcediag.ValueSnapshot
	c := NewClient(ClientOptions{ResourceDiagnostics: resourcediag.Observer{
		Fields: func(value resourcediag.Report) { report = value },
		Values: func(value resourcediag.ValueSnapshot) { snapshot = value },
	}})
	defer c.Close()
	if err := c.parseResource([]byte(`{"data":{"appList":{"data":{"appInfo":[{"apps":[{"displayName":"PRIVATE_NAME","id":"app-id","accessModel":"L3VPN","addressList":[{"protocol":"tcp","port":"443","host":"app.example"}]}]}]}}}}`)); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(report)
	if !strings.Contains(string(data), ".displayName") || strings.Contains(string(data), "PRIVATE_NAME") || strings.Contains(string(data), "app.example") {
		t.Fatalf("bad diagnostic: %s", data)
	}
	if snapshot.Err != nil || !strings.Contains(string(snapshot.Data), "PRIVATE_NAME") || !strings.Contains(string(snapshot.Data), "app.example") {
		t.Fatal("value diagnostic did not retain the unparsed resource field")
	}
	if _, ok := client.MatchDomainResource(c.domainResources["app.example"], "tcp", 443); !ok {
		t.Fatal("diagnostic changed resource rules")
	}
}

func TestParseResourcePreservesAddrPretendForDomain(t *testing.T) {
	policy := []byte(`{"data":{"appList":{"data":{"appInfo":[{"apps":[{"ID":"tcp-app","NodeGroupID":"tcp-group","AccessModel":"L3VPN","addrPretend":true,"AddressList":[{"Protocol":"tcp","Port":"80","Host":"speedtest.zju.edu.cn"}]}]}]}}}}`)
	c := &Client{}
	if err := c.parseResource(policy); err != nil {
		t.Fatalf("parseResource() error = %v", err)
	}

	resource, ok := client.MatchDomainResource(c.domainResources["speedtest.zju.edu.cn"], "tcp", 80)
	if !ok || !resource.AddrPretend {
		t.Fatalf("domain resource = (%#v, %t), want AddrPretend", resource, ok)
	}
}

func TestParseResourceAddrPretendDefaultsTrueAndOnlyAcceptsBool(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
		want  bool
	}{
		{name: "absent", want: true},
		{name: "null is ignored", field: `,"addrPretend":null`, want: true},
		{name: "explicit true", field: `,"addrPretend":true`, want: true},
		{name: "explicit false", field: `,"addrPretend":false`, want: false},
		{name: "zero number is ignored", field: `,"addrPretend":0`, want: true},
		{name: "nonzero number is ignored", field: `,"addrPretend":1`, want: true},
		{name: "string is ignored", field: `,"addrPretend":"false"`, want: true},
		{name: "object is ignored", field: `,"addrPretend":{}`, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := []byte(`{"data":{"appList":{"data":{"appInfo":[{"apps":[{"ID":"app","AccessModel":"L3VPN"` + test.field + `,"AddressList":[{"Protocol":"tcp","Port":"80","Host":"example.com"}]}]}]}}}}`)
			c := &Client{}
			if err := c.parseResource(policy); err != nil {
				t.Fatalf("parseResource() error = %v", err)
			}
			resource, ok := client.MatchDomainResource(c.domainResources["example.com"], "tcp", 80)
			if !ok || resource.AddrPretend != test.want {
				t.Fatalf("AddrPretend = (%t, %t), want %t", resource.AddrPretend, ok, test.want)
			}
		})
	}
}

func TestParseResourceDoesNotOverrideTCPTunnelZeroRTT(t *testing.T) {
	c := &Client{tcpTunnelZeroRTT: true}
	policy := []byte(`{"data":{"sdpPolicy":{"data":{"clientOption":{"tun0rtt":{"enable":false}}}}}}`)
	if err := c.parseResource(policy); err != nil {
		t.Fatalf("parseResource() error = %v", err)
	}
	if !c.tcpTunnelZeroRTT {
		t.Fatal("SDP tunnel pool policy overrode the manifest zero-RTT capability")
	}
}

func TestParseResourcePreservesTCPPrefL3ForDomainAndResolvedIP(t *testing.T) {
	var policy ClientResource
	policy.Data.AppList.Data.AppInfo = append(policy.Data.AppList.Data.AppInfo, resourceGroup{
		Apps: []resourceApp{{
			ID: "l3-app", NodeGroupID: "l3-group", AccessModel: "L3VPN", EnableTCPPrefL3: true,
			AddressList: []resourceAddress{{Protocol: "tcp", Port: "443", Host: "internal.example", IP: []string{"10.0.0.42"}}},
		}},
	})
	resource, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	c := &Client{}
	if err := c.parseResource(resource); err != nil {
		t.Fatalf("parseResource() error = %v", err)
	}
	domainResource, ok := client.MatchDomainResource(c.domainResources["internal.example"], "tcp", 443)
	if !ok || !domainResource.EnableTCPPrefL3 {
		t.Fatalf("domain resource = (%#v, %t), want TCP-prefers-L3", domainResource, ok)
	}
	ipResource, ok := c.resourceIndex.Match(net.ParseIP("10.0.0.42"), "tcp", 443)
	if !ok || !ipResource.EnableTCPPrefL3 || ipResource.AppID != "l3-app" {
		t.Fatalf("resolved IP resource = (%#v, %t), want l3-app TCP-prefers-L3", ipResource, ok)
	}
}

func TestDomainTCPSelectionPrefersTCPTunnelResource(t *testing.T) {
	resources := []client.DomainResource{
		{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "l3-app", EnableTCPPrefL3: true},
		{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "tcp-app"},
	}
	resource, ok := client.MatchDomainResourceWhere(resources, "tcp", 443, func(resource client.DomainResource) bool {
		return !resource.EnableTCPPrefL3
	})
	if !ok || resource.AppID != "tcp-app" {
		t.Fatalf("TCP tunnel domain match = (%#v, %t), want tcp-app", resource, ok)
	}
}

func TestParseResourceIgnoresUnsupportedAccessModel(t *testing.T) {
	var policy ClientResource
	policy.Data.AppList.Data.AppInfo = append(policy.Data.AppList.Data.AppInfo, resourceGroup{
		Apps: []resourceApp{{
			ID: "web-app", AccessModel: "WEB", AddressList: []resourceAddress{{Protocol: "tcp", Port: "443", Host: "10.0.0.42"}},
		}},
	})
	resource, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	c := &Client{}
	if err := c.parseResource(resource); err != nil {
		t.Fatalf("parseResource() error = %v", err)
	}
	if _, ok := c.resourceIndex.Match(net.ParseIP("10.0.0.42"), "tcp", 443); ok {
		t.Fatal("unsupported access model unexpectedly became an L3VPN resource")
	}
}
