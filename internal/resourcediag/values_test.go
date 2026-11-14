package resourcediag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/mythologyli/zju-connect/internal/bounded"
)

func TestATrustValueSnapshotPreservesGroupsAppsAndUnknownValues(t *testing.T) {
	var apps []string
	for i := range 12 {
		apps = append(apps, fmt.Sprintf(`{"name":"应用%d","description":"说明 & 100%%+","futureLabel":"未知标签","id":9007199254740993,"enabled":false,"optional":null,"addressList":[{"host":"app%d.internal","port":"443","protocol":"tcp"}],"ext":{"vpn.private.example":"动态字段值"}}`, i, i))
	}
	branch := `[{"name":"资源分组","workspaceName":"工作空间","apps":[` + strings.Join(apps, ",") + `]}]`
	input := []byte(`{"session":"OUTSIDE_SESSION","data":{"sdpPolicy":{"token":"OUTSIDE_POLICY"},"appList":{"config":"OUTSIDE_CONFIG","data":{"appInfo":` + branch + `}}}}`)
	snapshot := ATrustValues(input)
	if snapshot.Err != nil || snapshot.Format != "json" || bytes.Contains(snapshot.Data, []byte("OUTSIDE_")) {
		t.Fatalf("bad resource snapshot: %v", snapshot.Err)
	}
	var result struct {
		ScopeFound    bool            `json:"scope_found"`
		ValuesOmitted bool            `json:"values_omitted"`
		Truncated     bool            `json:"truncated"`
		CapturedAt    string          `json:"captured_at"`
		Resources     json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(snapshot.Data, &result); err != nil {
		t.Fatal(err)
	}
	decode := func(data []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !result.ScopeFound || result.ValuesOmitted || result.Truncated || result.CapturedAt == "" || !reflect.DeepEqual(decode(result.Resources), decode([]byte(branch))) {
		t.Fatal("resource hierarchy, values, types or numeric precision changed")
	}
	if !ATrust(input).Truncated {
		t.Fatal("fixture must exceed the field-only diagnostic sample limit")
	}
}

func TestECValueSnapshotPreservesAttributesTextAndAllEntries(t *testing.T) {
	doc := etree.NewDocument()
	xml := `<Resource token="OUTSIDE_AUTH"><Rcs>` + strings.Repeat(`<Rc name="财务 &amp; OA 100%+" host="app.internal" future="未知值"><Description>  保留空格 &amp; 内容  </Description></Rc>`, 12) + `</Rcs><Session value="OUTSIDE_SESSION"/></Resource>`
	if err := doc.ReadFromString(xml); err != nil {
		t.Fatal(err)
	}
	before, _ := doc.WriteToString()
	snapshot := EasyConnectValues(doc.Root().SelectElement("Rcs"))
	after, _ := doc.WriteToString()
	if snapshot.Err != nil || snapshot.Format != "xml" || before != after || bytes.Contains(snapshot.Data, []byte("OUTSIDE_")) {
		t.Fatalf("snapshot changed resource tree or scope: %v", snapshot.Err)
	}
	parsed := etree.NewDocument()
	if err := parsed.ReadFromBytes(snapshot.Data); err != nil {
		t.Fatal(err)
	}
	wrapper := parsed.Root()
	entries := wrapper.FindElements("Rcs/Rc")
	if wrapper.SelectAttrValue("values_omitted", "") != "false" || wrapper.SelectAttrValue("scope_found", "") != "true" || wrapper.SelectAttrValue("captured_at", "") == "" || len(entries) != 12 {
		t.Fatal("missing metadata or sampled resource values")
	}
	for _, entry := range entries {
		if entry.SelectAttrValue("name", "") != "财务 & OA 100%+" || entry.SelectAttrValue("future", "") != "未知值" || entry.SelectElement("Description").Text() != "  保留空格 & 内容  " {
			t.Fatal("attribute or text values changed")
		}
	}
}

func TestValueSnapshotsHandleLargeDeepAndMissingResources(t *testing.T) {
	for _, branch := range []string{
		`[{"name":"` + strings.Repeat("长", 400000) + `"}]`,
		strings.Repeat("[", 40) + `"nested value"` + strings.Repeat("]", 40),
		`[{"name":"{[\"escaped\\\" braces]}","more":"preserved"}]`,
	} {
		snapshot := ATrustValues([]byte(`{"data":{"appList":{"data":{"appInfo":` + branch + `}}}}`))
		if snapshot.Err != nil || !json.Valid(snapshot.Data) || len(snapshot.Data) > maxValueOutput {
			t.Fatalf("large/deep resource snapshot: %v", snapshot.Err)
		}
	}
	if got := ATrustValues(bytes.Repeat([]byte(" "), bounded.MaxResponse+1)); !errors.Is(got.Err, bounded.ErrTooLarge) || len(got.Data) != 0 {
		t.Fatal("oversized response accepted")
	}
	if got := ATrustValues([]byte(`{"invalid":`)); got.Err == nil || len(got.Data) != 0 {
		t.Fatal("malformed response exported")
	}
	if got := ATrustValues([]byte(`{"data":{}}`)); got.Err != nil || !bytes.Contains(got.Data, []byte(`"scope_found": false`)) || !bytes.Contains(got.Data, []byte(`"resources": null`)) {
		t.Fatal("missing scope not recorded")
	}
	if got := EasyConnectValues(nil); got.Err != nil || !bytes.Contains(got.Data, []byte(`scope_found="false"`)) {
		t.Fatal("missing XML scope not recorded")
	}
}
