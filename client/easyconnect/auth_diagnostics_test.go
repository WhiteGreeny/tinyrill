package easyconnect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/internal/authdiag"
)

func TestFailedLoginRetainsDiagnosticBeforeParsing(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Auth result="0"><ErrorCode>9876</ErrorCode><Message>Password expired PRIVATE_VALUE</Message></Auth>`))
	}))
	defer server.Close()
	var events []authdiag.Event
	c := NewClient(Options{Server: strings.TrimPrefix(server.URL, "https://"), Auth: AuthOptions{Diagnostics: func(e authdiag.Event) { events = append(events, e) }}})
	c.httpClient = server.Client()
	defer c.Close()
	// The response intentionally lacks TwfID; diagnostics must survive even when
	// the normal parser cannot proceed, without altering that failure.
	if err := c.loginAuthAndPsw(""); err == nil {
		t.Fatal("invalid login succeeded")
	}
	if len(events) != 1 || events[0].HTTPStatus != 403 || events[0].Endpoint != "/por/login_auth.csp" {
		t.Fatalf("lost auth response: %+v", events)
	}
}
