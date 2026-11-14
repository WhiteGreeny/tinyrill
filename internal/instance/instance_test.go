package instance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/internal/resourcediag"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/state"
)

func testProfile(t *testing.T) configs.Profile {
	t.Helper()
	p, err := configs.ParseProfile([]byte("schema_version: 1\nid: a\nprotocol: easyconnect\nserver: vpn.invalid\nauth:\n  username: user\n"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResourceDiagnosticsAreOptInAndSeparateFromStatus(t *testing.T) {
	p := testProfile(t)
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: p, Output: &out, Diagnostic: &diagnostic}
	if observer := f.ResourceDiagnostics(); observer.Fields != nil || observer.Values != nil {
		t.Fatal("diagnostic enabled by default")
	}
	directory, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	f.DebugResources = true
	if f.ResourceDiagnostics().Values != nil {
		t.Fatal("field-only diagnostic enabled raw values")
	}
	f.ResourceDiagnostics().ATrust([]byte(`{"data":{"appList":{"data":{"appInfo":[{"apps":[{"name":"PRIVATE_NAME"}]}]}}}}`))
	data, err := os.ReadFile(filepath.Join(p.Session.StateDir, "resource-fields.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report resourcediag.Report
	if err := json.Unmarshal(data, &report); err != nil || !report.ValuesOmitted || report.CapturedAt == "" || bytes.Contains(data, []byte("PRIVATE_NAME")) {
		t.Fatalf("bad diagnostic file: %s", data)
	}
	if out.Len() != 0 || !bytes.Contains(diagnostic.Bytes(), []byte("resource-fields.json")) {
		t.Fatal("diagnostic mixed with stdout or path not reported")
	}
	// A second capture replaces the old report instead of accumulating a log.
	f.ResourceDiagnostics().ATrust([]byte(`{"data":{}}`))
	data, _ = os.ReadFile(filepath.Join(p.Session.StateDir, "resource-fields.json"))
	if bytes.Contains(data, []byte("apps[].name")) {
		t.Fatal("old diagnostic content retained")
	}
}

func TestResourceValueDiagnosticsArePrivateLocalAndExplicit(t *testing.T) {
	p := testProfile(t)
	directory, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: p, Output: &out, Diagnostic: &diagnostic, DebugResourceValues: true}
	observer := f.ResourceDiagnostics()
	if observer.Fields == nil || observer.Values == nil {
		t.Fatal("value flag alone should enable both diagnostic files")
	}
	observer.ATrust([]byte(`{"session":"OUTSIDE_SESSION","data":{"appList":{"data":{"appInfo":[{"name":"RAW_GROUP","apps":[{"name":"RAW_APP","description":"RAW_DESCRIPTION","addressList":[{"host":"RAW_HOST"}]}]}]}}}}`))
	path := filepath.Join(p.Session.StateDir, "resource-values.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"RAW_GROUP", "RAW_APP", "RAW_DESCRIPTION", "RAW_HOST"} {
		if !bytes.Contains(data, []byte(value)) {
			t.Fatalf("missing value: %s", value)
		}
	}
	if bytes.Contains(data, []byte("OUTSIDE_SESSION")) || out.Len() != 0 || bytes.Contains(diagnostic.Bytes(), []byte("RAW_")) || !bytes.Contains(diagnostic.Bytes(), []byte(path)) {
		t.Fatal("raw values escaped the local resource file or saved path was not reported")
	}
	if err := securefile.Check(path); err != nil {
		t.Fatalf("diagnostic permissions: %v", err)
	}
	fields, err := os.ReadFile(filepath.Join(p.Session.StateDir, "resource-fields.json"))
	if err != nil || bytes.Contains(fields, []byte("RAW_")) {
		t.Fatal("field-only file gained resource values")
	}
	// Capture failure must leave the previous complete file intact.
	observer.Values(resourcediag.ValueSnapshot{Format: "json", Err: errors.New("RAW_ERROR")})
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) || bytes.Contains(diagnostic.Bytes(), []byte("RAW_ERROR")) {
		t.Fatal("failed capture changed the file or echoed a raw parser error")
	}
	observer.ATrust([]byte(`{"data":{}}`))
	after, _ = os.ReadFile(path)
	if bytes.Contains(after, []byte("RAW_")) || !bytes.Contains(after, []byte(`"scope_found": false`)) {
		t.Fatal("new capture did not replace old values")
	}
	if err := securefile.Check(path); err != nil {
		t.Fatal(err)
	}
	f.DebugResourceValues = false
	if next := f.ResourceDiagnostics(); next.Fields != nil || next.Values != nil {
		t.Fatal("value capture remained enabled")
	}
}

func TestResourceValueWriteFailureDoesNotEchoValues(t *testing.T) {
	p := testProfile(t)
	directory, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	blocked := filepath.Join(p.Session.StateDir, "resource-values.json")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	var out, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: p, Output: &out, Diagnostic: &diagnostic, DebugResourceValues: true}
	f.ResourceDiagnostics().ATrust([]byte(`{"data":{"appList":{"data":{"appInfo":[{"name":"RAW_PRIVATE"}]}}}}`))
	if out.Len() != 0 || bytes.Contains(diagnostic.Bytes(), []byte("RAW_PRIVATE")) || bytes.Contains(diagnostic.Bytes(), []byte("Resource values saved")) {
		t.Fatal("write failure echoed values or reported success")
	}
}

func TestRunDetectsStateAndPortConflictsBeforeAuthentication(t *testing.T) {
	p := testProfile(t)
	f := &CLIFrontend{Profile: p, Output: io.Discard, Diagnostic: io.Discard, Prompt: func(context.Context, string) (string, error) {
		t.Fatal("prompt before startup validation")
		return "", nil
	}}
	d, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(t.Context(), p, Material{}, f); !errors.Is(err, state.ErrLocked) {
		t.Fatalf("state conflict: %v", err)
	}
	d.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p.Listen.HTTP = l.Addr().String()
	var failure *Failure
	if err := Run(t.Context(), p, Material{}, f); !errors.As(err, &failure) || failure.Code != 2 {
		t.Fatalf("port conflict: %v", err)
	}
}

func TestRememberDisabledDoesNotReadOrWriteVault(t *testing.T) {
	p := testProfile(t)
	p.Credentials.Backend = "age-file"
	p.Credentials.VaultFile = filepath.Join(t.TempDir(), "not-a-vault.age")
	unchanged := []byte("unread-invalid-vault")
	if err := securefile.Write(p.Credentials.VaultFile, unchanged); err != nil {
		t.Fatal(err)
	}
	prompts := 0
	f := &CLIFrontend{Profile: p, Output: io.Discard, Diagnostic: io.Discard, Prompt: func(context.Context, string) (string, error) { prompts++; return "one-time-password", nil }}
	defer f.Close()
	if value, err := f.Password(t.Context()); err != nil || value != "one-time-password" {
		t.Fatalf("password: %v", err)
	}
	f.Authenticated(t.Context())
	if prompts != 1 {
		t.Fatalf("prompts = %d", prompts)
	}
	actual, _ := os.ReadFile(p.Credentials.VaultFile)
	if !bytes.Equal(actual, unchanged) {
		t.Fatal("disabled remember-password changed vault")
	}
}

func TestAuthenticationFailureDoesNotCreateVault(t *testing.T) {
	p := testProfile(t)
	p.Auth.RememberPassword = true
	p.Credentials.Backend = "age-file"
	p.Credentials.VaultFile = filepath.Join(t.TempDir(), "password.age")
	f := &CLIFrontend{Profile: p, Output: io.Discard, Diagnostic: io.Discard, Prompt: func(context.Context, string) (string, error) { return "unverified-password", nil }}
	if _, err := f.Password(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.Close() // No successful authentication event.
	if _, err := os.Stat(p.Credentials.VaultFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unverified password was persisted")
	}
}

func TestSecretFilePreservesSpacesAndRejectsMultipleLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	securefile.Write(path, []byte(" secret with spaces \r\n"))
	if value, err := ReadSecret(path); err != nil || value != " secret with spaces " {
		t.Fatalf("secret whitespace changed: %v", err)
	}
	securefile.Write(path, []byte("first\nsecond"))
	if _, err := ReadSecret(path); err == nil {
		t.Fatal("multiline secret accepted")
	}
}
