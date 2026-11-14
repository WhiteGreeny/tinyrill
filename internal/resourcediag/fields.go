// Package resourcediag provides opt-in diagnostics scoped to resource branches.
// Field reports omit values; a separate opt-in snapshot retains resource values.
package resourcediag

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/beevik/etree"
	"github.com/mythologyli/zju-connect/internal/bounded"
)

const (
	maxFields = 128
	maxNodes  = 512
	maxDepth  = 8
	maxItems  = 8
	maxPath   = 256
)

type Field struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Protocol      string  `json:"protocol"`
	Scope         string  `json:"scope"`
	ScopeFound    bool    `json:"scope_found"`
	CapturedAt    string  `json:"captured_at,omitempty"`
	ValuesOmitted bool    `json:"values_omitted"`
	Truncated     bool    `json:"truncated"`
	FilteredNames bool    `json:"filtered_names"`
	Fields        []Field `json:"fields"`
}

type Observer struct {
	Fields func(Report)
	Values func(ValueSnapshot)
}

func (o Observer) EasyConnect(root *etree.Element) {
	if o.Fields != nil {
		o.Fields(EasyConnect(root))
	}
	if o.Values != nil {
		o.Values(EasyConnectValues(root))
	}
}

func (o Observer) ATrust(resource []byte) {
	if o.Fields != nil {
		o.Fields(ATrust(resource))
	}
	if o.Values != nil {
		o.Values(ATrustValues(resource))
	}
}

type collector struct {
	report Report
	seen   map[Field]bool
	nodes  int
}

func newCollector(protocol, scope string) *collector {
	return &collector{report: Report{SchemaVersion: 1, Protocol: protocol, Scope: scope, ValuesOmitted: true, Fields: []Field{}}, seen: make(map[Field]bool)}
}

func (c *collector) enter(depth int) bool {
	if depth > maxDepth || c.nodes >= maxNodes {
		c.report.Truncated = true
		return false
	}
	c.nodes++
	return true
}

func (c *collector) add(path, kind string) {
	if len(path) > maxPath {
		c.report.Truncated = true
		return
	}
	field := Field{path, kind}
	if c.seen[field] {
		return
	}
	if len(c.report.Fields) >= maxFields {
		c.report.Truncated = true
		return
	}
	c.seen[field] = true
	c.report.Fields = append(c.report.Fields, field)
}

// Restrict output to ordinary schema identifiers. In particular, URLs, IP map
// keys, control characters and long dynamic keys are not exported as fields.
func (c *collector) fieldName(name string) bool {
	valid := len(name) > 0 && len(name) <= 64
	for i, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || i > 0 && (r >= '0' && r <= '9' || r == '-')) {
			valid = false
		}
	}
	if !valid {
		c.report.FilteredNames = true
	}
	return valid
}

func (c *collector) finish() Report {
	slices.SortFunc(c.report.Fields, func(a, b Field) int { return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Kind, b.Kind)) })
	return c.report
}

// EasyConnect inspects only the resource directory, after the existing bounded
// XML parser has accepted the document. Attribute values, text and comments are
// never copied into the report.
func EasyConnect(root *etree.Element) Report {
	c := newCollector("easyconnect", "Resource/Rcs")
	if root == nil {
		return c.finish()
	}
	c.report.ScopeFound = true
	c.xml(root, c.report.Scope, 0)
	return c.finish()
}

func (c *collector) xml(element *etree.Element, path string, depth int) {
	if !c.enter(depth) {
		return
	}
	c.add(path, "element")
	for i, attr := range element.Attr {
		if i >= maxFields {
			c.report.Truncated = true
			break
		}
		if c.fieldName(attr.Key) {
			c.add(path+"/@"+attr.Key, "attribute")
		}
	}
	if strings.TrimSpace(element.Text()) != "" {
		c.add(path+"/#text", "text")
	}
	children := element.ChildElements()
	counts := make(map[string]int)
	for _, child := range children {
		if c.nodes >= maxNodes {
			c.report.Truncated = true
			break
		}
		if !c.fieldName(child.Tag) {
			continue
		}
		counts[child.Tag]++
		if counts[child.Tag] > maxItems {
			c.report.Truncated = true
			continue
		}
		c.xml(child, path+"/"+child.Tag, depth+1)
	}
}

// ATrust deliberately decodes only appInfo. Authentication and SDP policy
// fields elsewhere in the resource response do not enter the collector.
func ATrust(resource []byte) Report {
	c := newCollector("atrust", "data.appList.data.appInfo")
	if len(resource) > bounded.MaxResponse {
		c.report.Truncated = true
		return c.finish()
	}
	appInfo, err := atrustAppInfo(resource)
	if err != nil || len(appInfo) == 0 {
		return c.finish()
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(appInfo))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return c.finish()
	}
	c.report.ScopeFound = true
	c.json(value, c.report.Scope, 0)
	return c.finish()
}

func (c *collector) json(value any, path string, depth int) {
	if !c.enter(depth) {
		return
	}
	switch value := value.(type) {
	case map[string]any:
		c.add(path, "object")
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if c.nodes >= maxNodes {
				c.report.Truncated = true
				break
			}
			if c.fieldName(key) {
				c.json(value[key], path+"."+key, depth+1)
			}
		}
	case []any:
		c.add(path, "array")
		if len(value) > maxItems {
			c.report.Truncated = true
		}
		for _, item := range value[:min(len(value), maxItems)] {
			c.json(item, path+"[]", depth+1)
		}
	case string:
		c.add(path, "string")
	case json.Number:
		c.add(path, "number")
	case bool:
		c.add(path, "boolean")
	case nil:
		c.add(path, "null")
	}
}
