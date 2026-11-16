package dnshosts

import (
	"fmt"
	"strings"
	"testing"
)

func TestExactAndLongestSuffixMatchOnlyDNSBoundaries(t *testing.T) {
	input := map[string]string{
		"+.example.internal":         "10.0.0.1",
		"+.apps.example.internal":    "10.0.0.2",
		"API.apps.example.internal.": "10.0.0.3",
	}
	table, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	input["+.example.internal"] = "10.99.0.1"
	for host, want := range map[string]string{
		"example.internal": "10.0.0.1", "a.b.example.internal": "10.0.0.1",
		"apps.example.internal": "10.0.0.2", "a.b.apps.example.internal": "10.0.0.2",
		"API.APPS.EXAMPLE.INTERNAL.": "10.0.0.3", "x.api.apps.example.internal": "10.0.0.2",
		"notexample.internal": "", "example.internal.evil": "", "a..example.internal": "",
		"app/example.internal": "", "app.\nexample.internal": "",
	} {
		ip, ok := table.Lookup(host)
		if ok != (want != "") || (ok && ip.String() != want) {
			t.Errorf("Lookup(%q) = %v/%v, want %q", host, ip, ok, want)
		}
	}
}

func TestInvalidAndAmbiguousMappingsAreRejected(t *testing.T) {
	for _, entries := range []map[string]string{
		{"*.example.internal": "10.0.0.1"}, {"+.": "10.0.0.1"}, {"bad..internal": "10.0.0.1"},
		{"https://app.internal": "10.0.0.1"}, {" app.internal": "10.0.0.1"}, {"app.internal:80": "10.0.0.1"},
		{"10.0.0.1": "10.0.0.2"}, {"-app.internal": "10.0.0.1"}, {"app_.internal": "10.0.0.1"},
		{"应用.internal": "10.0.0.1"}, {strings.Repeat("a", 64) + ".internal": "10.0.0.1"},
		{"app.internal": "::1"}, {"app.internal": "::ffff:10.0.0.1"}, {"app.internal": "other.internal"},
		{"APP.internal": "10.0.0.1", "app.internal.": "10.0.0.2"},
		{"+.APP.internal": "10.0.0.1", "+.app.internal.": "10.0.0.2"},
	} {
		if _, err := New(entries); err == nil {
			t.Errorf("accepted invalid mappings: %v", entries)
		}
	}
	entries := make(map[string]string)
	for i := 0; i < MaxEntries+1; i++ {
		entries[fmt.Sprintf("app%d.internal", i)] = "10.0.0.1"
	}
	if _, err := New(entries); err == nil {
		t.Fatal("accepted unbounded mapping table")
	}
}
