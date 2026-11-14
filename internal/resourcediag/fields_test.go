package resourcediag

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

func encoded(t *testing.T, report Report) string {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ValuesOmitted {
		t.Fatal("diagnostic must omit all values")
	}
	return string(data)
}

func TestECFieldsRetainNamesWithoutResourceOrSecretValues(t *testing.T) {
	doc := etree.NewDocument()
	err := doc.ReadFromString(`<Resource token="AUTH_MARKER"><Rcs><Rc name="RESOURCE_MARKER" host="HOST_MARKER" port="443~443" proto="0" type="1" token="TOKEN_MARKER"><Description>DESCRIPTION_MARKER</Description><!-- COMMENT_MARKER --></Rc></Rcs></Resource>`)
	if err != nil {
		t.Fatal(err)
	}
	report := EasyConnect(doc.SelectElement("Resource").SelectElement("Rcs"))
	data := encoded(t, report)
	if !report.ScopeFound || !strings.Contains(data, "Resource/Rcs/Rc/@name") || !strings.Contains(data, "Resource/Rcs/Rc/Description/#text") {
		t.Fatalf("missing resource shape: %s", data)
	}
	if strings.Contains(data, "MARKER") || strings.Contains(data, "443") || strings.Contains(data, "Resource/@token") {
		t.Fatalf("resource values or out-of-scope fields leaked: %s", data)
	}
}

func TestATrustFieldsRetainTypesWithoutResourceOrSecretValues(t *testing.T) {
	input := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"groupName":"GROUP_MARKER","apps":[{"name":"RESOURCE_MARKER","id":"ID_MARKER","token":"TOKEN_MARKER","enabled":true,"rank":9,"optional":null,"addressList":[{"host":"HOST_MARKER","port":"443","protocol":"tcp"}]}]}]}},"sdpPolicy":{"privateSetting":"POLICY_MARKER"}},"cookie":"COOKIE_MARKER"}`)
	report := ATrust(input)
	data := encoded(t, report)
	for _, path := range []string{"appInfo[].groupName", "apps[].name", "addressList[].host"} {
		if !strings.Contains(data, path) {
			t.Errorf("missing %s", path)
		}
	}
	for _, kind := range []string{"object", "array", "string", "number", "boolean", "null"} {
		if !strings.Contains(data, `"kind":"`+kind+`"`) {
			t.Errorf("missing type %s", kind)
		}
	}
	if !report.ScopeFound || strings.Contains(data, "MARKER") || strings.Contains(data, "sdpPolicy") || strings.Contains(data, "cookie") || strings.Contains(data, "443") {
		t.Fatalf("unexpected diagnostic content: %s", data)
	}
	if second := encoded(t, ATrust(input)); data != second {
		t.Fatal("unstable field ordering")
	}
}

func TestFieldDiagnosticsAreBoundedAndFilterDynamicKeys(t *testing.T) {
	fields := map[string]any{"vpn.private.example": "PRIVATE_MARKER", "10.1.2.3": "PRIVATE_MARKER", "bad\nkey": "PRIVATE_MARKER", strings.Repeat("a", 1000): "PRIVATE_MARKER"}
	for i := range 300 {
		fields[fmt.Sprintf("field%d", i)] = "PRIVATE_MARKER"
	}
	root := map[string]any{"data": map[string]any{"appList": map[string]any{"data": map[string]any{"appInfo": []any{fields}}}}}
	input, _ := json.Marshal(root)
	report := ATrust(input)
	data := encoded(t, report)
	if !report.Truncated || !report.FilteredNames || len(report.Fields) > maxFields || len(data) > 64<<10 {
		t.Fatalf("unbounded diagnostic: %d fields, %d bytes", len(report.Fields), len(data))
	}
	if strings.Contains(data, "PRIVATE_MARKER") || strings.Contains(data, "vpn.private.example") || strings.Contains(data, "10.1.2.3") || strings.Contains(data, "bad") {
		t.Fatal("dynamic key or value leaked")
	}
	doc := etree.NewDocument()
	doc.ReadFromString(`<Rcs>` + strings.Repeat(`<Rc name="PRIVATE_MARKER"/>`, 20) + `</Rcs>`)
	if report := EasyConnect(doc.Root()); !report.Truncated || len(report.Fields) > maxFields {
		t.Fatal("XML sample limit not applied")
	}
	for _, input := range [][]byte{nil, []byte(`{"data":{}}`), []byte(`{"data":`)} {
		if ATrust(input).ScopeFound {
			t.Fatal("missing scope reported as found")
		}
	}
	if EasyConnect(nil).ScopeFound {
		t.Fatal("missing XML scope reported as found")
	}
}
