package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaintenanceOutlivesSetupContext(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "refreshed", Path: "/"})
		w.Write([]byte(`{"code":0,"data":{"isLogin":1}}`))
	}))
	defer server.Close()
	s := newTLSTestSession(server)
	setup, cancelSetup := context.WithCancel(context.Background())
	s.ctx = setup
	s.Restore("device", "original", nil)
	s.ContinueWithContext(t.Context())
	cancelSetup()
	result, err := s.Refresh(t.Context())
	if err != nil || result.SID != "refreshed" {
		t.Fatalf("maintenance still tied to setup: %v", err)
	}
}
