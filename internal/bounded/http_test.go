package bounded

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { return copy(p, `{"code":0}`), io.ErrUnexpectedEOF }
func TestResponseLimitsAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		reader io.Reader
		err    error
	}{
		{strings.NewReader(strings.Repeat("x", MaxResponse+1)), ErrTooLarge},
		{brokenReader{}, io.ErrUnexpectedEOF},
	} {
		r := &http.Response{Body: io.NopCloser(tc.reader), ContentLength: -1}
		if err := BufferResponse(r); !errors.Is(err, tc.err) {
			t.Fatalf("error=%v want %v", err, tc.err)
		}
	}
}
func TestAuthenticationRedirectStaysOnOrigin(t *testing.T) {
	first, _ := http.NewRequest("POST", "https://vpn.example/login", nil)
	for _, tc := range []struct {
		target string
		valid  bool
	}{
		{"https://vpn.example:443/next", true}, {"https://other.example/next", false},
		{"http://vpn.example/next", false}, {"https://vpn.example:444/next", false},
	} {
		next, _ := http.NewRequest("POST", tc.target, nil)
		if err := AuthRedirect(next, []*http.Request{first}); (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.target, err)
		}
	}
	secret := "never-log-query"
	err := TransportError(&url.Error{Op: "Post", URL: "https://vpn.example/?password=" + secret, Err: io.EOF})
	if !errors.Is(err, io.EOF) || strings.Contains(err.Error(), secret) {
		t.Fatal(err)
	}
}
func TestXMLBounds(t *testing.T) {
	for _, s := range []string{`<!DOCTYPE x><x/>`, strings.Repeat("<x>", 65) + strings.Repeat("</x>", 65), `<x>`} {
		if XML(s) == nil {
			t.Fatal("accepted invalid or unbounded XML")
		}
	}
	if err := XML(`<Resource><Rcs/></Resource>`); err != nil {
		t.Fatal(err)
	}
}
