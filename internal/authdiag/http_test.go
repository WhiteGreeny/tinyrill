package authdiag

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/internal/bounded"
)

func TestCaptureKeepsUsefulMetadataWithoutSecrets(t *testing.T) {
	for _, tc := range []struct{ protocol, path, body, codePath, servicePath string }{
		{"atrust", "/passport/v1/auth/psw", `{"code":9876,"message":"Password expired: VERY_PRIVATE 密码已过期，请修改","data":{"nextService":"auth/changePassword","graphCheckCodeEnable":0,"passwordExpired":true,"ticket":{"code":5678,"message":"VERY_PRIVATE"},"antiReplayRand":"VERY_PRIVATE","password":"VERY_PRIVATE","username":"VERY_PRIVATE","phone":13800138000,"unknown":"VERY_PRIVATE","odd":{"name":"VERY_PRIVATE"},"https://VERY_PRIVATE/":1}}`, "$.code", "$.data.nextService"},
		{"easyconnect", "/por/login_psw.csp", `<Auth result="0"><ErrorCode>9876</ErrorCode><Message>Password expired: VERY_PRIVATE 密码已过期，请修改</Message><NextService>auth/changePassword</NextService><NextAuth>8</NextAuth><TwfID>VERY_PRIVATE</TwfID><cookie><code>5678</code></cookie><unknown>VERY_PRIVATE</unknown><phone>13800138000</phone></Auth>`, "Auth/ErrorCode", "Auth/NextService"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			request, _ := http.NewRequest("POST", "https://VERY_PRIVATE"+tc.path+"?ticket=VERY_PRIVATE", strings.NewReader("password=VERY_PRIVATE"))
			request.Header.Set("Authorization", "VERY_PRIVATE")
			response := &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Set-Cookie": {"VERY_PRIVATE"}}}
			var captured Event
			Observer(func(e Event) { captured = e }).Capture(tc.protocol, request, response, nil)
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != tc.body {
				t.Fatal("diagnostic changed parser input")
			}
			data, _ := json.Marshal(captured)
			if bytes.Contains(data, []byte("VERY_PRIVATE")) || bytes.Contains(data, []byte("13800138000")) || bytes.Contains(data, []byte("5678")) {
				t.Fatalf("secret in diagnostic: %s", data)
			}
			if captured.Endpoint != tc.path || captured.HTTPStatus != 403 || captured.Outcome != "response" || captured.At.IsZero() {
				t.Fatalf("missing context: %+v", captured)
			}
			var code, service, signals bool
			for _, field := range captured.Fields {
				if field.Path == tc.codePath && field.Value == int64(9876) {
					code = true
				}
				if field.Path == tc.servicePath && field.Value == "auth/changePassword" {
					service = true
				}
				if len(field.Signals) > 0 {
					signals = true
				}
			}
			if !code || !service || !signals {
				t.Fatalf("missing diagnostic metadata: %s", data)
			}
		})
	}
}

func TestCaptureIsOptInAndLimitedToAuthentication(t *testing.T) {
	var calls int
	observer := Observer(func(Event) { calls++ })
	for _, path := range []string{"/por/rclist.csp", "/passport/v1/public/checkCode", "/passport/v1/auth/unknown/VERY_PRIVATE", "/controller/v1/public/reportEnv"} {
		req, _ := http.NewRequest("GET", "https://vpn.invalid"+path, nil)
		response := &http.Response{Body: io.NopCloser(strings.NewReader("unread"))}
		observer.Capture("atrust", req, response, nil)
		observer.Capture("easyconnect", req, response, nil)
		body, _ := io.ReadAll(response.Body)
		if string(body) != "unread" {
			t.Fatal("unrelated response touched")
		}
	}
	var disabled Observer
	req, _ := http.NewRequest("GET", "https://vpn.invalid/por/login_auth.csp", nil)
	disabled.Capture("easyconnect", req, nil, errors.New("VERY_PRIVATE"))
	if calls != 0 {
		t.Fatal("diagnostic enabled for unrelated traffic")
	}
}

func TestCaptureHandlesLargeAndFailedResponses(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://vpn.invalid/passport/v1/auth/psw", nil)
	for _, tc := range []struct {
		body      string
		err       error
		outcome   string
		truncated bool
	}{
		{strings.Repeat("x", maxBody+100), nil, "body_too_large", true},
		{"", errors.New("url with VERY_PRIVATE"), "request_failed", false},
		{"", bounded.ErrTooLarge, "response_too_large", true},
		{`{"message":"VERY_PRIVATE",`, nil, "invalid_body", false},
		{`<Auth><Message>VERY_PRIVATE</Auth>`, nil, "invalid_body", false},
		{"VERY_PRIVATE", nil, "unsupported_body", false},
		{"", nil, "empty_body", false},
	} {
		response := &http.Response{Body: io.NopCloser(strings.NewReader(tc.body))}
		var event Event
		Observer(func(e Event) { event = e }).Capture("atrust", req, response, tc.err)
		if event.Outcome != tc.outcome || event.Truncated != tc.truncated {
			t.Fatalf("unexpected result: %+v", event)
		}
		body, _ := io.ReadAll(response.Body)
		if string(body) != tc.body {
			t.Fatal("body changed")
		}
		data, _ := json.Marshal(event)
		if bytes.Contains(data, []byte("VERY_PRIVATE")) {
			t.Fatal("error or invalid body leaked")
		}
	}
}

func TestMetadataRejectsFreeTextAndNestedSecretValues(t *testing.T) {
	var e Event
	inspect(&e, []byte(`{"code":"12345678901234567890","success":false,"message":"unexpected VERY_PRIVATE","data":{"code":345678,"nextService":"https://VERY_PRIVATE/?token=VERY_PRIVATE","unknown":{"code":1234},"ticket":{"nextService":"auth/VERY_PRIVATE","message":"VERY_PRIVATE"}},"payload":{"code":234567,"nextService":"auth/VERY_PRIVATE"}}`))
	data, _ := json.Marshal(e)
	if bytes.Contains(data, []byte("VERY_PRIVATE")) || bytes.Contains(data, []byte("12345678901234567890")) || bytes.Contains(data, []byte(`"value":1234`)) {
		t.Fatalf("unsafe values: %s", data)
	}
	if !bytes.Contains(data, []byte(`"value":false`)) {
		t.Fatal("false metadata lost")
	}
	if bytes.Contains(data, []byte("234567")) || bytes.Contains(data, []byte("345678")) {
		t.Fatal("nested code mistaken for gateway result")
	}
	var xmlEvent Event
	inspect(&xmlEvent, []byte(`<Other><code>234567</code><NextService>auth/VERY_PRIVATE</NextService></Other>`))
	xmlData, _ := json.Marshal(xmlEvent)
	if bytes.Contains(xmlData, []byte("234567")) || bytes.Contains(xmlData, []byte("VERY_PRIVATE")) {
		t.Fatal("unknown XML envelope retained values")
	}
}

func TestStructureLimits(t *testing.T) {
	for _, body := range []string{
		`{"data":[` + strings.Repeat(`{"code":1},`, 10) + `{}]}`,
		strings.Repeat(`<a>`, maxDepth+3) + strings.Repeat(`</a>`, maxDepth+3),
		`<Auth>` + strings.Repeat(`<Message>expired</Message>`, maxFields+3) + `</Auth>`,
		`{"data":` + strings.Repeat(`{"a":`, maxDepth+3) + `1` + strings.Repeat(`}`, maxDepth+3) + `}`,
	} {
		var e Event
		inspect(&e, []byte(body))
		if !e.Truncated || len(e.Fields) > maxFields {
			t.Fatalf("unbounded diagnostic: %+v", e)
		}
	}
}

func FuzzInspect(f *testing.F) {
	f.Add([]byte(`{"code":0,"data":{"nextService":"auth/authCheck"}}`))
	f.Add([]byte(`<Auth result="0"><NextAuth>7</NextAuth></Auth>`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > maxBody {
			t.Skip()
		}
		var event Event
		inspect(&event, body)
		if len(event.Fields) > maxFields {
			t.Fatal("field limit exceeded")
		}
		for _, field := range event.Fields {
			if len(field.Path) > 256 {
				t.Fatal("path limit exceeded")
			}
		}
	})
}
