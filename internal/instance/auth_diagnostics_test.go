package instance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mythologyli/zju-connect/internal/authdiag"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/state"
)

func TestAuthDiagnosticIsPrivateBoundedAndIndependent(t *testing.T) {
	p := testProfile(t)
	var output, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: p, Output: &output, Diagnostic: &diagnostic, Version: "test-version", Commit: "test-commit"}
	path := filepath.Join(p.Session.StateDir, "auth-debug.json")
	if f.AuthDiagnostics(1) != nil {
		t.Fatal("enabled by default")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disabled diagnostic created a file")
	}
	dir, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	f.DebugAuth = true
	observer := f.AuthDiagnostics(1)
	var wg sync.WaitGroup
	for i := 0; i < maxAuthEvents+5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			observer(authdiag.Event{Endpoint: "/por/login_auth.csp", Outcome: "response"})
		}()
	}
	wg.Wait()
	f.AuthDiagnostics(2)(authdiag.Event{Endpoint: "/por/login_psw.csp", Outcome: "response"})
	if output.Len() != 0 || strings.Count(diagnostic.String(), path) != 1 {
		t.Fatal("diagnostic leaked to stdout or repeatedly announced")
	}
	f.Status(Status{State: "failed", Reason: "authentication_rejected"})
	if err := securefile.Check(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report authDiagnosticReport
	if json.Unmarshal(data, &report) != nil || len(report.Events) != maxAuthEvents || report.DroppedEvents != 6 || report.State != "failed" || report.Events[maxAuthEvents-1].Generation != 2 || report.Version != "test-version" {
		t.Fatalf("bad report: %s", data)
	}
	if bytes.Contains(output.Bytes(), []byte("events")) {
		t.Fatal("report mixed with status")
	}
	other := &CLIFrontend{Profile: p, DebugAuth: true, Diagnostic: io.Discard}
	other.AuthDiagnostics(1)
	data, _ = os.ReadFile(path)
	if json.Unmarshal(data, &report) != nil || len(report.Events) != 0 || report.DroppedEvents != 0 {
		t.Fatal("previous run retained")
	}
	// Turning the flag off does not remove evidence from an earlier run.
	disabled := &CLIFrontend{Profile: p}
	disabled.AuthDiagnostics(1)
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, data) {
		t.Fatal("disabled run changed old report")
	}
}

func TestAuthDiagnosticSurvivesFailedSetup(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`<Auth result="0"><Message>密码过期 PRIVATE_VALUE</Message><ErrorCode>9876</ErrorCode></Auth>`))
	}))
	defer server.Close()
	p := testProfile(t)
	p.Server = strings.TrimPrefix(server.URL, "https://")
	p.TLS.Mode = "insecure"
	p.Listen.HTTP, p.Listen.SOCKS5 = "127.0.0.1:0", ""
	var output, diagnostic bytes.Buffer
	f := &CLIFrontend{Profile: p, Output: &output, Diagnostic: &diagnostic, DebugAuth: true}
	defer f.Close()
	if err := Run(t.Context(), p, Material{}, f); err == nil {
		t.Fatal("bad authentication succeeded")
	}
	data, err := os.ReadFile(filepath.Join(p.Session.StateDir, "auth-debug.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report authDiagnosticReport
	if json.Unmarshal(data, &report) != nil || len(report.Events) == 0 || report.State != "failed" {
		t.Fatalf("failed authentication not retained: %s", data)
	}
	if !bytes.Contains(data, []byte("9876")) || bytes.Contains(data, []byte("PRIVATE_VALUE")) {
		t.Fatal("missing code or leaked value")
	}
}

func TestAuthDiagnosticWriteFailureDoesNotStopObserver(t *testing.T) {
	p := testProfile(t)
	dir, err := state.Acquire(p.Session.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := os.Mkdir(filepath.Join(p.Session.StateDir, "auth-debug.json"), 0700); err != nil {
		t.Fatal(err)
	}
	f := &CLIFrontend{Profile: p, DebugAuth: true, Output: io.Discard, Diagnostic: io.Discard}
	f.AuthDiagnostics(1)(authdiag.Event{Outcome: "request_failed"})
	f.Status(Status{State: "failed"})
	if len(f.authReport.Events) != 1 || !f.authWriteFailed {
		t.Fatal("writer failure disrupted capture")
	}
}
