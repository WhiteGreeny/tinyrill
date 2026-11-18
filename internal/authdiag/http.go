// Package authdiag collects opt-in authentication metadata, never raw messages,
// requests, headers or authentication secrets. It does not classify failures.
package authdiag

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mythologyli/zju-connect/internal/bounded"
)

const maxBody = 128 << 10

type Field struct {
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Value   any      `json:"value,omitempty"`
	Signals []string `json:"text_signals,omitempty"`
}

type Event struct {
	At            time.Time `json:"at"`
	Generation    uint64    `json:"generation"`
	Endpoint      string    `json:"endpoint"`
	HTTPStatus    int       `json:"http_status,omitempty"`
	Outcome       string    `json:"outcome"`
	Format        string    `json:"format,omitempty"`
	Truncated     bool      `json:"truncated"`
	FilteredNames bool      `json:"filtered_names"`
	Fields        []Field   `json:"fields,omitempty"`
}

type Observer func(Event)

// Capture runs after BufferResponse. Restoring the inspected prefix preserves
// the exact bytes the authentication parser will read, including large bodies.
// No additional request is made and nil observers do no work.
func (o Observer) Capture(protocol string, req *http.Request, resp *http.Response, requestErr error) {
	if o == nil || req == nil || req.URL == nil || !authenticationEndpoint(protocol, req.URL.Path) {
		return
	}
	e := Event{At: time.Now().UTC(), Endpoint: req.URL.Path, Outcome: "response"}
	if resp != nil {
		e.HTTPStatus = resp.StatusCode
	}
	if requestErr != nil {
		e.Outcome = "request_failed"
		if errors.Is(requestErr, bounded.ErrTooLarge) {
			e.Outcome = "response_too_large"
			e.Truncated = true
		}
	} else if resp != nil && resp.Body != nil {
		original := resp.Body
		prefix, err := io.ReadAll(io.LimitReader(original, maxBody+1))
		resp.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(prefix), original), Closer: original}
		switch {
		case err != nil:
			e.Outcome = "body_unavailable"
		case len(prefix) > maxBody:
			e.Outcome, e.Truncated = "body_too_large", true
		default:
			inspect(&e, prefix)
		}
	}
	o(e)
}

type replayedBody struct {
	io.Reader
	io.Closer
}

// Only fixed paths used by our authentication implementation are exported.
// A callback URL, redirect target or gateway-provided path is never logged.
func authenticationEndpoint(protocol, path string) bool {
	if protocol == "easyconnect" {
		switch path {
		case "/por/login_auth.csp", "/por/login_psw.csp", "/por/login_sms.csp", "/por/login_sms1.csp", "/por/login_token.csp", "/por/login_cert.csp", "/por/update_session.csp":
			return true
		}
	}
	if protocol == "atrust" {
		switch path {
		case "/passport/v1/public/authConfig", "/passport/v1/public/sendSms",
			"/passport/v1/auth/psw", "/passport/v1/auth/authCheck", "/passport/v1/auth/sms", "/passport/v1/auth/customSms",
			"/passport/v1/auth/smsCheckCode", "/passport/v1/auth/token", "/passport/v1/auth/challenge",
			"/passport/v1/auth/accessCheck", "/passport/v1/auth/bindAuthDevice", "/passport/v1/auth/preEnhancedAuth",
			"/passport/v1/auth/enhancedConfirm", "/passport/v1/auth/enhancedDone", "/passport/v1/auth/cas", "/passport/v1/auth/httpsOauth2",
			"/passport/v1/user/onlineInfo":
			return true
		}
	}
	return false
}
