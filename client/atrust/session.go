package atrust

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mythologyli/zju-connect/client/atrust/auth"
	"github.com/mythologyli/zju-connect/log"
)

func (c *Client) sessionSID() (string, error) {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.SID, c.sessionErr
}

func (c *Client) setSessionSID(sid string, err error) {
	c.sessionMu.Lock()
	c.SID, c.sessionErr = sid, err
	c.sessionMu.Unlock()
}

func (c *Client) startSessionRefresh(refresh func(context.Context) (auth.LoginResult, error), data auth.ClientAuthData, save func([]byte) error, interval time.Duration) {
	c.maintenanceMu.Lock()
	defer c.maintenanceMu.Unlock()
	if c.lifecycleCtx.Err() != nil || c.refreshDone != nil {
		return
	}
	done := make(chan struct{})
	c.refreshDone = done
	go func() {
		defer close(done)
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-c.lifecycleCtx.Done():
				return
			case <-timer.C:
			}
			snapshot, err := refresh(c.lifecycleCtx)
			if c.lifecycleCtx.Err() != nil {
				return
			}
			if errors.Is(err, auth.ErrSessionInvalid) {
				c.reportFailure(err)
				return
			}
			if err != nil {
				log.Printf("aTrust authConfig refresh failed: %v", err)
				timer.Reset(min(interval, time.Minute))
				continue
			}
			oldSID, _ := c.sessionSID()
			c.setSessionSID(snapshot.SID, nil)
			log.Printf("aTrust session refreshed (SID changed: %t)", oldSID != snapshot.SID)
			data.Cookies = snapshot.Cookies
			if save != nil {
				encoded, err := json.Marshal(data)
				if err == nil {
					err = save(encoded)
				}
				if err != nil {
					log.Printf("Failed to save refreshed aTrust cookies: %v", err)
				}
			}
			timer.Reset(interval)
		}
	}()
}
