package authdiag

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	maxFields = 128
	maxDepth  = 12
	maxNodes  = 1024
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
var serviceName = regexp.MustCompile(`^auth/[A-Za-z][A-Za-z0-9_]{0,47}$`)
var englishSignals = regexp.MustCompile(`(?i)\b(password|expired|expire|expiration|expiry|change|reset|mandatory|required|first|login|days)\b`)

func inspect(e *Event, body []byte) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		e.Outcome = "empty_body"
		return
	}
	c := collector{event: e}
	switch body[0] {
	case '{', '[':
		e.Format = "json"
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
			e.Outcome = "invalid_body"
			return
		}
		c.json(value, "$", "", 0)
	case '<':
		e.Format = "xml"
		c.xml(body)
	default:
		e.Outcome = "unsupported_body"
	}
}

type collector struct {
	event *Event
	nodes int
}

func (c *collector) add(path, name, kind string, value any, depth int) {
	if len(c.event.Fields) >= maxFields || len(path) > 256 {
		c.event.Truncated = true
		return
	}
	f := Field{Path: path, Kind: kind}
	if sensitive(name) {
		f.Kind = "redacted"
	} else {
		// Only shallow, explicitly allowed metadata can retain values. Unknown
		// fields, user IDs, times, arbitrary numbers and strings remain omitted.
		if depth <= 2 {
			f.Value = metadata(name, value)
		}
		if text, ok := value.(string); ok && messageField(name) {
			f.Signals = signals(text)
		}
	}
	c.event.Fields = append(c.event.Fields, f)
}

func sensitive(name string) bool {
	name = strings.ToLower(name)
	// Metadata names are checked separately: passwordExpired is a flag, not
	// the password. Subtrees of secret-bearing fields are never inspected.
	switch name {
	case "password", "passwd", "pwd", "psw", "secret", "username", "account", "phone", "phonenumber", "email", "deviceid", "macaddresses", "sid", "twfid", "csrf_rand_code", "antireplayrand", "rsa_encrypt_key", "pubkey", "env", "rid", "graphcheckcode", "checkcode", "otp":
		return true
	}
	for _, part := range []string{"cookie", "token", "ticket", "secret", "csrf", "nonce", "signkey", "authorization", "credential"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func metadata(name string, value any) any {
	name = strings.ToLower(name)
	if name == "nextservice" {
		if s, ok := value.(string); ok && serviceName.MatchString(strings.TrimSpace(s)) {
			return strings.TrimSpace(s)
		}
		return nil
	}
	switch name {
	case "code", "result", "errorcode", "errcode", "nextauth", "graphcheckcodeenable":
		var s string
		switch v := value.(type) {
		case json.Number:
			s = string(v)
		case string:
			s = strings.TrimSpace(v)
		default:
			return nil
		}
		if len(s) <= 11 {
			if n, err := strconv.ParseInt(s, 10, 32); err == nil {
				return n
			}
		}
	case "success", "passwordexpired", "pwdexpired", "needchangepassword", "forcechangepassword", "mustchangepassword":
		if v, ok := value.(bool); ok {
			return v
		}
		s := strings.TrimSpace(valueString(value))
		if s == "0" || s == "1" || s == "true" || s == "false" {
			return s
		}
	}
	return nil
}

func valueString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return string(v)
	}
	return ""
}

func messageField(name string) bool {
	switch strings.ToLower(name) {
	case "message", "msg", "error", "errormsg", "errormessage", "reason", "description", "tip", "tips":
		return true
	}
	return false
}

// Signals are fixed vocabulary matches, not extracts or an expiry verdict.
// Raw server prose can echo passwords even when it has no secret field name.
func signals(text string) []string {
	seen := map[string]bool{}
	for _, match := range englishSignals.FindAllString(text, -1) {
		seen[strings.ToLower(match)] = true
	}
	for _, word := range []string{"密码", "口令", "过期", "到期", "失效", "修改", "重置", "首次", "强制", "天"} {
		if strings.Contains(text, word) {
			seen[word] = true
		}
	}
	result := make([]string, 0, len(seen))
	for word := range seen {
		result = append(result, word)
	}
	slices.Sort(result)
	return result
}

func (c *collector) name(name string) bool {
	if !identifier.MatchString(name) {
		c.event.FilteredNames = true
		return false
	}
	return true
}

func (c *collector) enter(depth int) bool {
	c.nodes++
	if depth > maxDepth || c.nodes > maxNodes || len(c.event.Fields) >= maxFields {
		c.event.Truncated = true
		return false
	}
	return true
}

func (c *collector) json(value any, path, name string, depth int) {
	if !c.enter(depth) {
		return
	}
	if sensitive(name) {
		c.add(path, name, "redacted", nil, depth)
		return
	}
	switch v := value.(type) {
	case map[string]any:
		c.add(path, name, "object", nil, depth)
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b string) int {
			// Keep error and step metadata before potentially large config trees.
			priority := func(s string) bool {
				return metadata(s, "0") != nil || strings.EqualFold(s, "nextService") || messageField(s)
			}
			if priority(a) != priority(b) {
				if priority(a) {
					return -1
				}
				return 1
			}
			return strings.Compare(a, b)
		})
		for _, key := range keys {
			if c.name(key) {
				c.json(v[key], path+"."+key, key, depth+1)
			}
			if len(c.event.Fields) >= maxFields {
				c.event.Truncated = true
				break
			}
		}
	case []any:
		c.add(path, name, "array", nil, depth)
		for i, item := range v {
			if i >= 4 {
				c.event.Truncated = true
				break
			}
			c.json(item, path+"["+strconv.Itoa(i)+"]", "", depth+1)
		}
	case string:
		c.add(path, name, "string", v, depth)
	case json.Number:
		c.add(path, name, "number", v, depth)
	case bool:
		c.add(path, name, "boolean", v, depth)
	default:
		c.add(path, name, "null", nil, depth)
	}
}

func (c *collector) xml(body []byte) {
	type node struct {
		path, name, text string
		hidden           bool
		children         bool
		overflow         bool
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var stack []node
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			c.event.Outcome = "invalid_body"
			return
		}
		c.nodes++
		if c.nodes > maxNodes || len(c.event.Fields) >= maxFields {
			c.event.Truncated = true
			return
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= maxDepth {
				c.event.Truncated = true
				return
			}
			n := node{name: t.Name.Local, path: t.Name.Local}
			n.hidden = !c.name(n.name)
			if len(stack) > 0 {
				parent := &stack[len(stack)-1]
				parent.children = true
				n.path = parent.path + "/" + n.name
				n.hidden = n.hidden || parent.hidden || sensitive(parent.name)
			}
			if !n.hidden {
				for _, attr := range t.Attr {
					if c.name(attr.Name.Local) && !sensitive(n.name) {
						c.add(n.path+"/@"+attr.Name.Local, attr.Name.Local, "attribute", attr.Value, len(stack)+1)
					}
				}
			}
			stack = append(stack, n)
		case xml.CharData:
			if len(stack) > 0 {
				n := &stack[len(stack)-1]
				if !n.hidden && !sensitive(n.name) {
					if len(n.text)+len(t) <= 4096 {
						n.text += string(t)
					} else {
						c.event.Truncated = true
						n.overflow = true
					}
				}
			}
		case xml.EndElement:
			if len(stack) == 0 {
				c.event.Outcome = "invalid_body"
				return
			}
			n := stack[len(stack)-1]
			if !n.hidden {
				var value any
				if !n.children && !n.overflow {
					value = n.text
				}
				c.add(n.path, n.name, "element", value, len(stack)-1)
			}
			stack = stack[:len(stack)-1]
		case xml.Directive:
			c.event.Outcome = "unsupported_body"
			return
		}
	}
}
