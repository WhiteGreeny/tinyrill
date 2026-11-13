package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type blockingRoundTripper struct{}

func (blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestNewSessionContextCancelsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSessionContext(ctx, "vpn.example", nil)
	session.client.Transport = blockingRoundTripper{}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := session.request(req)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not observe session cancellation")
	}
}

// The bounded response is fully read before return and remains readable after
// the network request has completed.
func TestRequestKeepsBodyReadableAfterReturn(t *testing.T) {
	const head, tail = `{"code":0,`, `"data":{}}`
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, head)
		w.(http.Flusher).Flush()
		<-release // the body is still in flight when request() returns
		_, _ = io.WriteString(w, tail)
	}))
	defer server.Close()
	defer unblock()

	session := newTLSTestSession(server)
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, session.baseURL+"/passport/v1/public/authConfig", nil)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		resp *http.Response
		err  error
	}
	done := make(chan result, 1)
	go func() { resp, err := session.request(req); done <- result{resp, err} }()
	select {
	case <-done:
		t.Fatal("request returned an incomplete body")
	case <-time.After(25 * time.Millisecond):
	}
	unblock()
	got := <-done
	resp, err := got.resp, got.err
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != head+tail {
		t.Fatalf("body = %q, want %q", body, head+tail)
	}
}
