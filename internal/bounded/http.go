// Package bounded limits untrusted gateway input before protocol parsing.
package bounded

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const MaxResponse = 8 << 20

var ErrTooLarge = errors.New("gateway response exceeds size limit")

func TransportError(err error) error {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return fmt.Errorf("%s gateway request: %w", urlError.Op, urlError.Err)
	}
	return err
}

func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// BufferResponse eagerly authenticates and bounds the entire HTTP body, also
// when the transport decompresses it. Callers cannot ignore a read error and
// accidentally parse a valid-looking prefix of a truncated response.
func BufferResponse(response *http.Response) error {
	body := response.Body
	defer body.Close()
	if response.ContentLength > MaxResponse {
		return ErrTooLarge
	}
	data, err := ReadAll(body, MaxResponse)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	return nil
}

func origin(u *url.URL) string {
	port := u.Port()
	if port == "" && u.Scheme == "https" {
		port = "443"
	}
	return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

// Authentication redirects may retain a POST body. Never send those bodies to
// another origin, or downgrade HTTPS, based on a server-controlled Location.
func AuthRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("too many authentication redirects")
	}
	if len(via) == 0 {
		return nil
	}
	if request.URL.Scheme != "https" || origin(request.URL) != origin(via[0].URL) {
		return errors.New("authentication redirect changed origin")
	}
	return nil
}
