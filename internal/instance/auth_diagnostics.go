package instance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mythologyli/zju-connect/internal/authdiag"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/terminal"
	"github.com/mythologyli/zju-connect/log"
)

const maxAuthEvents = 64

type authDiagnosticReport struct {
	SchemaVersion int              `json:"schema_version"`
	Protocol      string           `json:"protocol"`
	Version       string           `json:"version"`
	Commit        string           `json:"commit"`
	StartedAt     time.Time        `json:"started_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	ValuePolicy   string           `json:"value_policy"`
	State         string           `json:"state"`
	Reason        string           `json:"reason,omitempty"`
	DroppedEvents uint64           `json:"dropped_events"`
	Events        []authdiag.Event `json:"events"`
}

// Run calls this only after acquiring the per-customer state lock. A new run
// replaces the old report; within a run a bounded tail also covers session checks.
func (f *CLIFrontend) AuthDiagnostics(generation uint64) authdiag.Observer {
	if !f.DebugAuth {
		return nil
	}
	f.authMu.Lock()
	if f.authReport == nil {
		f.authReport = &authDiagnosticReport{SchemaVersion: 1, Protocol: f.Profile.Protocol, Version: f.Version, Commit: f.Commit, StartedAt: time.Now().UTC(), ValuePolicy: "allowlisted_metadata_only", State: "starting", Events: []authdiag.Event{}}
		f.saveAuthDiagnostic()
	}
	f.authMu.Unlock()
	return func(event authdiag.Event) {
		f.authMu.Lock()
		defer f.authMu.Unlock()
		event.Generation = generation
		if len(f.authReport.Events) == maxAuthEvents {
			copy(f.authReport.Events, f.authReport.Events[1:])
			f.authReport.Events = f.authReport.Events[:maxAuthEvents-1]
			f.authReport.DroppedEvents++
		}
		f.authReport.Events = append(f.authReport.Events, event)
		f.saveAuthDiagnostic()
	}
}

// Caller holds authMu. Only sanitized reports enter this writer; write errors
// never include response data and cannot change the authentication result.
func (f *CLIFrontend) saveAuthDiagnostic() {
	f.authReport.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(f.authReport, "", "  ")
	path := filepath.Join(f.Profile.Session.StateDir, "auth-debug.json")
	if err == nil {
		err = securefile.Write(path, append(data, '\n'))
	}
	if err != nil {
		if !f.authWriteFailed {
			log.Println("Unable to save authentication diagnostic; VPN setup will continue")
		}
		f.authWriteFailed = true
		return
	}
	f.authWriteFailed = false
	if !f.authNotice && f.Diagnostic != nil {
		fmt.Fprintf(f.Diagnostic, "Authentication diagnostic saved (limited metadata; review before sharing): %s\n", terminal.SafeText(path))
		f.authNotice = true
	}
}
