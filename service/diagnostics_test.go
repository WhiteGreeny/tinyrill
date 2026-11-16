package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
)

func captureDiagnostics(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := stdlog.Writer()
	stdlog.SetOutput(&output)
	t.Cleanup(func() { stdlog.SetOutput(previous) })
	return &output
}

func TestSOCKSFailuresReportDNSAndAuthorizationWithoutDebug(t *testing.T) {
	output := captureDiagnostics(t)
	log.DisableDebug()
	r := resolve.NewResolver(nil, "", "", 60, nil, nil, false)
	defer r.Close()
	s, err := Reserve("", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Start(dial.NewDialer(nil, r, nil), r, time.Second)
	for _, target := range []string{"missing.internal", "missing.internal", "10.0.0.1"} {
		conn, err := net.Dial("tcp", s.socks.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		conn.Write([]byte{5, 1, 0})
		var greeting [2]byte
		if _, err := io.ReadFull(conn, greeting[:]); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		request := []byte{5, 1, 0, 3, byte(len(target))}
		if ip := net.ParseIP(target); ip != nil {
			request = append([]byte{5, 1, 0, 1}, ip.To4()...)
		} else {
			request = append(request, target...)
		}
		request = append(request, 1, 187)
		conn.Write(request)
		var reply [10]byte
		_, err = io.ReadFull(conn, reply[:])
		conn.Close()
		if err != nil || reply[1] == 0 {
			t.Fatalf("expected rejected request: reply=%v err=%v", reply, err)
		}
	}
	s.Close()
	text := output.String()
	if strings.Count(text, "stage=dns") != 1 || !strings.Contains(text, `target="missing.internal"`) || !strings.Contains(text, "no VPN DNS") {
		t.Fatalf("missing/rate-unlimited DNS failure: %s", text)
	}
	if !strings.Contains(text, "stage=authorization") || !strings.Contains(text, `target="10.0.0.1:443"`) {
		t.Fatalf("missing authorization failure: %s", text)
	}
}

func TestFailureLoggingIsBoundedAndPerInstance(t *testing.T) {
	now := time.Unix(100, 0)
	l := failureLog{now: func() time.Time { return now }}
	key := failureKey{"dns", "app.internal", "unavailable"}
	if ok, _ := l.allow(key); !ok {
		t.Fatal("first failure suppressed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.allow(key); ok {
				t.Error("repeated failure logged")
			}
		}()
	}
	wg.Wait()
	other := failureLog{}
	if ok, _ := other.allow(key); !ok {
		t.Fatal("another instance inherited suppression")
	}
	now = now.Add(failureLogInterval)
	if ok, count := l.allow(key); !ok || count != 100 {
		t.Fatalf("window/count = %v/%d", ok, count)
	}
	for i := 0; i < failureLogCapacity+20; i++ {
		l.allow(failureKey{"dns", fmt.Sprint(i), "unavailable"})
	}
	if len(l.entries) > failureLogCapacity {
		t.Fatalf("unbounded log state: %d", len(l.entries))
	}
}

func TestFailureLogRedactsSecretsAndIgnoresCancellation(t *testing.T) {
	output := captureDiagnostics(t)
	l := failureLog{}
	l.report("dial", "app.internal:443", errors.New("password=PRIVATE_PASSWORD token=PRIVATE_TOKEN\n\x1b[31m"))
	l.report("dial", "cancelled.internal:443", context.Canceled)
	text := output.String()
	if strings.Contains(text, "PRIVATE_") || strings.Contains(text, "\x1b") || strings.Contains(text, "cancelled.internal") {
		t.Fatalf("unsafe/noisy diagnostic: %s", text)
	}
}
