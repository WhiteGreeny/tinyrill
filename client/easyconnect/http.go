package easyconnect

import (
	"github.com/mythologyli/zju-connect/internal/bounded"
	"net/http"
)

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, bounded.TransportError(err)
	}
	if err := bounded.BufferResponse(resp); err != nil {
		return nil, err
	}
	return resp, nil
}
