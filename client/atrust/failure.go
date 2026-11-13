package atrust

import (
	"errors"
	"github.com/mythologyli/zju-connect/client/atrust/auth"
)

func (c *Client) Errors() <-chan error { return c.failures }

func (c *Client) reportFailure(err error) {
	if errors.Is(err, auth.ErrSessionInvalid) {
		c.setSessionSID("", err)
	}
	select {
	case c.failures <- err:
	default:
	}
	if c.lifecycleCancel != nil {
		c.lifecycleCancel()
	}
}
