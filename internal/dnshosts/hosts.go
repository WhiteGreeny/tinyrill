// Package dnshosts compiles per-customer static DNS mappings. It grants no access.
package dnshosts

import (
	"errors"
	"net/netip"
	"sort"
	"strings"
)

const MaxEntries = 256

type suffixEntry struct {
	name string
	ip   netip.Addr
}

// Table owns an immutable copy of the configuration and is safe for concurrent use.
type Table struct {
	exact    map[string]netip.Addr
	suffixes []suffixEntry
}

func New(entries map[string]string) (*Table, error) {
	if len(entries) > MaxEntries {
		return nil, errors.New("at most 256 static DNS mappings are supported")
	}
	table := &Table{exact: make(map[string]netip.Addr)}
	seen := make(map[string]bool)
	for pattern, address := range entries {
		name := strings.TrimSuffix(strings.ToLower(pattern), ".")
		suffix := strings.HasPrefix(name, "+.")
		if suffix {
			name = strings.TrimPrefix(name, "+.")
		}
		if !validName(name) {
			return nil, errors.New("static DNS keys must be ASCII hostnames or +.suffix patterns")
		}
		if _, err := netip.ParseAddr(name); err == nil {
			return nil, errors.New("static DNS keys must be domain names, not IP addresses")
		}
		key := name
		if suffix {
			key = "+." + name
		}
		if seen[key] {
			return nil, errors.New("duplicate static DNS pattern after hostname normalization")
		}
		seen[key] = true
		ip, err := netip.ParseAddr(address)
		if err != nil || !ip.Is4() {
			return nil, errors.New("static DNS values must be IPv4 addresses")
		}
		if suffix {
			table.suffixes = append(table.suffixes, suffixEntry{name, ip})
		} else {
			table.exact[name] = ip
		}
	}
	sort.Slice(table.suffixes, func(i, j int) bool {
		if len(table.suffixes[i].name) != len(table.suffixes[j].name) {
			return len(table.suffixes[i].name) > len(table.suffixes[j].name)
		}
		return table.suffixes[i].name < table.suffixes[j].name
	})
	return table, nil
}

func (t *Table) Lookup(host string) (netip.Addr, bool) {
	if t == nil {
		return netip.Addr{}, false
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if !validName(host) {
		return netip.Addr{}, false
	}
	if ip, ok := t.exact[host]; ok {
		return ip, true
	}
	for _, entry := range t.suffixes {
		if host == entry.name || strings.HasSuffix(host, "."+entry.name) {
			return entry.ip, true
		}
	}
	return netip.Addr{}, false
}

func validName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, b := range []byte(label) {
			if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '-' {
				return false
			}
		}
	}
	return true
}
