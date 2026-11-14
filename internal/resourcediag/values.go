package resourcediag

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/beevik/etree"
	"github.com/mythologyli/zju-connect/internal/bounded"
)

// ValueSnapshot contains an entire resource branch, including unknown fields.
// It must only be sent to a private local file after explicit user opt-in.
type ValueSnapshot struct {
	Format string
	Data   []byte
	Err    error
}

const maxValueOutput = 64 << 20

func atrustAppInfo(resource []byte) (json.RawMessage, error) {
	if len(resource) > bounded.MaxResponse {
		return nil, bounded.ErrTooLarge
	}
	var envelope struct {
		Data struct {
			AppList struct {
				Data struct{ AppInfo json.RawMessage }
			}
		}
	}
	err := json.Unmarshal(resource, &envelope)
	return envelope.Data.AppList.Data.AppInfo, err
}

// ATrustValues retains the original JSON types, numbers, unknown fields and
// group/app relationships. It does not select a name field or sample entries.
func ATrustValues(resource []byte) ValueSnapshot {
	appInfo, err := atrustAppInfo(resource)
	if err != nil {
		return ValueSnapshot{Format: "json", Err: err}
	}
	envelope := struct {
		SchemaVersion int             `json:"schema_version"`
		Protocol      string          `json:"protocol"`
		Scope         string          `json:"scope"`
		ScopeFound    bool            `json:"scope_found"`
		CapturedAt    string          `json:"captured_at"`
		ValuesOmitted bool            `json:"values_omitted"`
		Truncated     bool            `json:"truncated"`
		Resources     json.RawMessage `json:"resources"`
	}{SchemaVersion: 1, Protocol: "atrust", Scope: "data.appList.data.appInfo", ScopeFound: len(appInfo) != 0, CapturedAt: time.Now().UTC().Format(time.RFC3339), Resources: appInfo}
	data, err := json.Marshal(envelope)
	// Avoid indentation amplification for large or deeply nested gateway data.
	// The fallback is complete compact JSON, never a truncated value sample.
	if err == nil && len(data) <= 1<<20 && shallowJSON(data, 16) {
		var pretty bytes.Buffer
		err = json.Indent(&pretty, data, "", "  ")
		data = pretty.Bytes()
	}
	if len(data) >= maxValueOutput {
		err = bounded.ErrTooLarge
	}
	if err != nil {
		return ValueSnapshot{Format: "json", Err: err}
	}
	return ValueSnapshot{Format: "json", Data: append(data, '\n')}
}

// Input has already passed JSON validation. Braces inside strings do not count.
func shallowJSON(data []byte, limit int) bool {
	depth, inString, escaped := 0, false, false
	for _, b := range data {
		if inString {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > limit {
				return false
			}
		case '}', ']':
			depth--
		}
	}
	return true
}

// EasyConnectValues receives only Rcs after the client's bounded XML parser.
// Keep the subtree intact, including attributes/text that are not routing data.
func EasyConnectValues(root *etree.Element) ValueSnapshot {
	doc := etree.NewDocument()
	wrapper := doc.CreateElement("ResourceDiagnostic")
	wrapper.CreateAttr("schema_version", "1")
	wrapper.CreateAttr("protocol", "easyconnect")
	wrapper.CreateAttr("scope", "Resource/Rcs")
	wrapper.CreateAttr("scope_found", strconv.FormatBool(root != nil))
	wrapper.CreateAttr("captured_at", time.Now().UTC().Format(time.RFC3339))
	wrapper.CreateAttr("values_omitted", "false")
	wrapper.CreateAttr("truncated", "false")
	if root != nil {
		wrapper.AddChild(root.Copy())
	}
	// Do not reindent XML: whitespace can itself be a resource field value.
	var output valueBuffer
	_, err := doc.WriteTo(&output)
	return ValueSnapshot{Format: "xml", Data: output.Bytes(), Err: err}
}

type valueBuffer struct{ bytes.Buffer }

func (b *valueBuffer) Write(data []byte) (int, error) {
	if len(data) > maxValueOutput-b.Len() {
		return 0, bounded.ErrTooLarge
	}
	return b.Buffer.Write(data)
}
