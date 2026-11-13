package authchallenge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/internal/terminal"
)

func TestSessionRejectsStaleIdentityAndExpiry(t *testing.T) {
	for _, field := range []string{"customer", "generation", "id", "expiry"} {
		t.Run(field, func(t *testing.T) {
			h := NewSessionHandler(SessionOptions{Customer: "a", Generation: 2, Timeout: 20 * time.Millisecond, Responder: ResponderFunc(func(ctx context.Context, c Challenge) (Response, error) {
				r := Response{Meta: c.Meta, Value: "123"}
				switch field {
				case "customer":
					r.Meta.Customer = "b"
				case "generation":
					r.Meta.Generation--
				case "id":
					r.Meta.ID = "old"
				case "expiry":
					<-ctx.Done()
				}
				return r, nil
			})})
			if _, err := h.HandleCodeChallenge(CodeChallenge{Kind: CodeSMS}); err == nil {
				t.Fatal("stale response accepted")
			}
		})
	}
}

func TestSessionOnlyOnePendingChallenge(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := NewSessionHandler(SessionOptions{Responder: ResponderFunc(func(ctx context.Context, c Challenge) (Response, error) {
		close(started)
		<-release
		return Response{Meta: c.Meta, Value: "123"}, nil
	})})
	go func() { defer close(done); _, _ = h.HandleCodeChallenge(CodeChallenge{}) }()
	<-started
	if _, err := h.HandleCodeChallenge(CodeChallenge{}); err == nil {
		t.Fatal("parallel challenges accepted")
	}
	close(release)
	<-done
}

func TestNonInteractiveAndOversizeInputFail(t *testing.T) {
	h := NewCLIHandler(CLIOptions{Never: true, Input: strings.NewReader("secret\n"), Output: &bytes.Buffer{}})
	if _, err := h.HandleCodeChallenge(CodeChallenge{}); !errors.Is(err, terminal.ErrInteractionRequired) {
		t.Fatalf("noninteractive: %v", err)
	}
	h = NewCLIHandler(CLIOptions{Input: strings.NewReader(strings.Repeat("x", terminal.MaxInput+1)), Output: &bytes.Buffer{}})
	if _, err := h.HandleCodeChallenge(CodeChallenge{}); err == nil {
		t.Fatal("oversized response accepted")
	}
}

func TestCaptchaCallbackRejectsCSRFAndReplays(t *testing.T) {
	c, err := newCaptchaCallback(Challenge{Meta: Metadata{Customer: "a", ID: "challenge-a", Expires: time.Now().Add(time.Minute)}, Image: testImage()}, "http://127.0.0.1:28000")
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"id":"challenge-a","coordinates":[[10,20]],"width":300,"height":150}`
	send := func(path, host, origin, body string) int {
		r := httptest.NewRequest(http.MethodPost, "http://"+host+path, strings.NewReader(body))
		r.Host = host
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r)
		return w.Code
	}
	for _, bad := range []struct{ path, host, origin, body string }{
		{c.basePath + "submit", "evil.example:28000", c.origin, valid},
		{c.basePath + "submit", c.host, "http://evil.example", valid},
		{c.basePath + "submit", c.host, "", valid},
		{"/c/wrong/submit", c.host, c.origin, valid},
		{c.basePath + "submit", c.host, c.origin, strings.Replace(valid, "challenge-a", "old-challenge", 1)},
		{c.basePath + "submit", c.host, c.origin, strings.Replace(valid, "[10,20]", "[310,20]", 1)},
		{c.basePath + "submit", c.host, c.origin, valid + strings.Repeat(" ", 17<<10)},
	} {
		if code := send(bad.path, bad.host, bad.origin, bad.body); code == 200 {
			t.Fatal("unsafe callback accepted")
		}
	}
	if code := send(c.basePath+"submit", c.host, c.origin, valid); code != 200 {
		t.Fatalf("valid callback: %d", code)
	}
	result := <-c.result
	if result.err != nil || len(result.response.Points) != 1 {
		t.Fatal("missing response")
	}
	if code := send(c.basePath+"submit", c.host, c.origin, valid); code == 200 {
		t.Fatal("replayed callback accepted")
	}
}

func TestCaptchaPageContainsNoExternalResourcesAndExpires(t *testing.T) {
	c, _ := newCaptchaCallback(Challenge{Meta: Metadata{Customer: "<script>alert(1)</script>", ID: "id", Expires: time.Now().Add(time.Minute)}, Image: testImage()}, "http://127.0.0.1:28000")
	r := httptest.NewRequest(http.MethodGet, c.origin+c.basePath, nil)
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("unsafe or invalid page")
	}
	if !strings.Contains(w.Body.String(), `id:"id"`) {
		t.Fatal("challenge id not bound to browser response")
	}
	c.challenge.Meta.Expires = time.Now().Add(-time.Second)
	w = httptest.NewRecorder()
	c.ServeHTTP(w, r)
	if w.Code != http.StatusGone {
		t.Fatal("expired challenge served")
	}
}

func TestCaptchaImageLimits(t *testing.T) {
	if _, err := imageSize(make([]byte, maxImageBytes+1)); err == nil {
		t.Fatal("oversize image accepted")
	}
	if _, err := imageSize([]byte("<svg onload='alert(1)'/>")); err == nil {
		t.Fatal("active image accepted")
	}
	var points ClickCaptchaResponse
	_ = json.Unmarshal([]byte(`{}`), &points)
	if err := validateImagePoints(testImage(), points); err == nil {
		t.Fatal("empty points accepted")
	}
}
