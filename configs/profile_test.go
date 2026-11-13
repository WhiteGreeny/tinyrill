package configs

import (
	"path/filepath"
	"strings"
	"testing"
)

const validProfile = "schema_version: 1\nid: customer-a\nprotocol: easyconnect\nserver: VPN.Example.:0443\n"

func TestProfileDefaultsAndRelativeReferences(t *testing.T) {
	base := t.TempDir()
	p, err := ParseProfile([]byte(validProfile+"auth:\n  username: alice\n  secret_file: secrets/login\n"), base)
	if err != nil {
		t.Fatal(err)
	}
	if p.Server != "vpn.example:443" || p.Auth.RememberPassword {
		t.Fatalf("unsafe defaults: %+v", p)
	}
	if p.Listen.SOCKS5 != "127.0.0.1:1080" || p.Listen.HTTP != "127.0.0.1:1081" {
		t.Fatal("listeners must default to loopback")
	}
	if p.Auth.SecretFile != filepath.Join(base, "secrets/login") || p.Session.StateDir != filepath.Join(base, "state/customer-a") {
		t.Fatal("relative paths did not use configuration directory")
	}
}

func TestProfileRejectsAmbiguousAndSecretInput(t *testing.T) {
	for name, input := range map[string]string{
		"unknown":            validProfile + "unknown: true\n",
		"duplicate":          validProfile + "id: another\n",
		"password":           validProfile + "auth:\n  password: PRIVATE_VALUE\n",
		"number string":      validProfile + "auth:\n  username: 1234\n",
		"string bool":        validProfile + "auth:\n  remember_password: 'false'\n",
		"null":               validProfile + "auth: null\n",
		"tag":                validProfile + "auth: !custom {}\n",
		"anchor":             validProfile + "auth: &auth {}\n",
		"multiple documents": validProfile + "---\n{}\n",
		"version":            strings.Replace(validProfile, "schema_version: 1", "schema_version: 2", 1),
		"server URL":         strings.Replace(validProfile, "VPN.Example.:0443", "https://alice:PRIVATE_VALUE@vpn.example/", 1),
		"public fallback":    validProfile + "dns:\n  mode: system\n",
		"too large":          validProfile + strings.Repeat("# x\n", MaxConfigBytes),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseProfile([]byte(input), t.TempDir())
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), "PRIVATE_VALUE") {
				t.Fatalf("configuration error exposed secret: %v", err)
			}
		})
	}
}

func TestAuthInfoDoesNotRequireLoginCredentials(t *testing.T) {
	p, err := ParseProfile([]byte(validProfile), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if p.ValidateForRun() == nil {
		t.Fatal("run accepted missing username")
	}
}
