package easyconnect

import (
	"github.com/mythologyli/zju-connect/internal/bounded"
	"net/http"
)

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.authDiagnostics.Capture("easyconnect", req, resp, err)
		return nil, bounded.TransportError(err)
	}
	err = bounded.BufferResponse(resp)
	c.authDiagnostics.Capture("easyconnect", req, resp, err)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
