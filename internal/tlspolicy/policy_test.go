package tlspolicy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTLSRequiresExplicitTrust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	for _, test := range []struct {
		name    string
		options Options
		success bool
	}{
		{"unknown CA", Options{}, false},
		{"customer CA", Options{CAFile: ca}, true},
		{"wrong hostname", Options{CAFile: ca, ServerName: "wrong.invalid"}, false},
		{"trusted pin", Options{Mode: "pin", Pins: []string{"sha256/" + base64.StdEncoding.EncodeToString(pin[:])}}, true},
		{"wrong pin", Options{Mode: "pin", Pins: []string{"sha256/" + base64.StdEncoding.EncodeToString(make([]byte, 32))}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := New(test.options)
			if err != nil {
				t.Fatal(err)
			}
			transport := &http.Transport{TLSClientConfig: p.Standard()}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			response, err := client.Get(server.URL)
			if response != nil {
				response.Body.Close()
			}
			if (err == nil) != test.success {
				t.Fatalf("TLS result: %v, want success %v", err, test.success)
			}
		})
	}
}
