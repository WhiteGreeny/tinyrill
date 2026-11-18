package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/internal/authdiag"
)

func TestPasswordFailureDiagnosticDoesNotChangeAuthentication(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/passport/v1/auth/psw" {
			t.Error("unexpected endpoint")
		}
		fmt.Fprint(w, `{"code":9876,"message":"Password expired PRIVATE_VALUE","data":{"nextService":"auth/changePassword","ticket":"PRIVATE_VALUE"}}`)
	}))
	defer server.Close()
	session := newTLSTestSession(server)
	defer session.Close()
	session.pubKey, session.pubKeyExp = key.PublicKey.N.Text(16), strconv.Itoa(key.PublicKey.E)
	var events []authdiag.Event
	session.SetDiagnostics(func(e authdiag.Event) { events = append(events, e) })
	_, err = session.pswImpl("PRIVATE_VALUE", "PRIVATE_VALUE", "PRIVATE_VALUE", "")
	if !errors.Is(err, client.ErrAuthenticationRejected) || len(events) != 1 {
		t.Fatalf("changed authentication result: %v, events %d", err, len(events))
	}
	data, _ := json.Marshal(events)
	if !strings.Contains(string(data), "9876") || !strings.Contains(string(data), "auth/changePassword") || strings.Contains(string(data), "PRIVATE_VALUE") {
		t.Fatalf("incorrect diagnostic: %s", data)
	}
}
