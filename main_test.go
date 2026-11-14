package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOnlyAcceptsExplicitYAMLCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "customer.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 1\nid: a\nprotocol: easyconnect\nserver: vpn.example\nauth:\n  username: user\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"version"}, 0}, {[]string{"config-check", "--config", path}, 0},
		{[]string{"run", "--config", path, "--password", "PASSWORD_MARKER"}, 2},
		{[]string{"run", "--config", path, "--tun-mode"}, 2},
		{[]string{"run", "--config", path, "--resources", "PASSWORD_MARKER"}, 2},
		{[]string{"config-check", "--config", path, "--resources", "full"}, 2},
		{[]string{"credentials", "print", "--config", path}, 2},
		{[]string{"run"}, 2},
	} {
		var out, diagnostic bytes.Buffer
		code := runCLI(t.Context(), test.args, &out, &diagnostic)
		if code != test.code {
			t.Fatalf("%s: code %d, want %d", test.args[0], code, test.code)
		}
		if strings.Contains(out.String()+diagnostic.String(), "PASSWORD_MARKER") {
			t.Fatal("argument value leaked into output")
		}
	}
}

func TestConfigCheckRejectsPlaintextWithoutEcho(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(path, []byte("schema_version: 1\nid: a\nprotocol: atrust\nserver: vpn.example\nauth:\n  password: SECRET_MARKER\n"), 0600)
	var out, diagnostic bytes.Buffer
	if code := runCLI(t.Context(), []string{"config-check", "--config", path}, &out, &diagnostic); code != 2 {
		t.Fatalf("code %d", code)
	}
	if strings.Contains(diagnostic.String(), "SECRET_MARKER") {
		t.Fatal("secret exposed")
	}
}
