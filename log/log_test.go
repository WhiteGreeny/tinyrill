package log

import (
	"bytes"
	stdlog "log"
	"strings"
	"testing"
)

func TestDebugPrintfHonorsEnabledState(t *testing.T) {
	var output bytes.Buffer
	previous := stdlog.Writer()
	stdlog.SetOutput(&output)
	t.Cleanup(func() {
		DisableDebug()
		stdlog.SetOutput(previous)
	})

	DisableDebug()
	DebugPrintf("hidden %d", 1)
	if output.Len() != 0 {
		t.Fatalf("disabled debug output = %q, want empty", output.String())
	}

	EnableDebug()
	DebugPrintf("visible %d", 2)
	if !strings.Contains(output.String(), "visible 2") {
		t.Fatalf("enabled debug output = %q, want visible message", output.String())
	}
}

func TestSecretsOmittedInNormalAndDebugLogs(t *testing.T) {
	var out bytes.Buffer
	old := stdlog.Writer()
	stdlog.SetOutput(&out)
	defer func() { stdlog.SetOutput(old); DisableDebug() }()
	for _, enabled := range []bool{false, true} {
		if enabled {
			EnableDebug()
		} else {
			DisableDebug()
		}
		Printf("password=%s Cookie=%s SID=%s SignKey=%s", "secret-password", "secret-cookie", "secret-sid", "secret-signkey")
		DebugPrintf("token=%s https://user:secret-user@vpn.example/auth?code=secret-query", "secret-token")
		DumpHex([]byte("secret-payload"))
		DebugDumpHex([]byte("secret-debug-payload"))
	}
	Println("untrusted\x1b[31m\nforged")
	if strings.Contains(out.String(), "secret-") || strings.Contains(out.String(), "\x1b") {
		t.Fatalf("leaked data: %s", out.String())
	}
}
