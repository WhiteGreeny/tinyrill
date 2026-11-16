package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/internal/bounded"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
)

const failureLogInterval = 30 * time.Second
const failureLogCapacity = 256

type failureKey struct{ stage, target, reason string }
type failureWindow struct {
	last       time.Time
	suppressed uint64
}

type reportedFailure struct{ error }

func (e *reportedFailure) Unwrap() error { return e.error }

// Each instance bounds its own diagnostic state; no timers or global target cache.
type failureLog struct {
	mu      sync.Mutex
	entries map[failureKey]failureWindow
	now     func() time.Time
}

func (l *failureLog) report(stage, target string, err error) {
	if err == nil {
		return
	}
	var lookup *resolve.LookupError
	if errors.As(err, &lookup) {
		stage, target = "dns", lookup.Host
	} else if errors.Is(err, dial.ErrACLDenied) {
		stage = "authorization"
	} else if errors.Is(err, dial.ErrSessionUnavailable) {
		stage = "session"
	}
	target = diagnosticText(target, 512)
	reason := diagnosticText(bounded.TransportError(err).Error(), 1024)
	if errors.Is(err, context.Canceled) {
		log.DebugPrintf("Proxy cancelled: stage=%s target=%q error=%s", stage, target, reason)
		return
	}
	suppressed := uint64(0)
	if !log.DebugEnabled() {
		var allowed bool
		allowed, suppressed = l.allow(failureKey{stage, target, reason})
		if !allowed {
			return
		}
	}
	log.Printf("Proxy failure: stage=%s target=%q error=%s suppressed=%d", stage, target, reason, suppressed)
}

func (l *failureLog) allow(key failureKey) (bool, uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	if l.entries == nil {
		l.entries = make(map[failureKey]failureWindow)
	}
	window, found := l.entries[key]
	if found && now.Sub(window.last) < failureLogInterval {
		window.suppressed++
		l.entries[key] = window
		return false, 0
	}
	if !found && len(l.entries) >= failureLogCapacity {
		var oldestKey failureKey
		var oldestTime time.Time
		for candidate, entry := range l.entries {
			if oldestTime.IsZero() || entry.last.Before(oldestTime) {
				oldestKey, oldestTime = candidate, entry.last
			}
		}
		delete(l.entries, oldestKey)
	}
	l.entries[key] = failureWindow{last: now}
	return true, window.suppressed
}

func diagnosticText(value string, limit int) string {
	value = log.Redact(value)
	if len(value) > limit {
		value = value[:limit] + "..."
	}
	return strings.ToValidUTF8(value, "?")
}
