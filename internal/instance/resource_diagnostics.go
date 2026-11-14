package instance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mythologyli/zju-connect/internal/resourcediag"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/terminal"
	"github.com/mythologyli/zju-connect/log"
)

func (f *CLIFrontend) ResourceDiagnostics() resourcediag.Observer {
	var observer resourcediag.Observer
	if f.DebugResources || f.DebugResourceValues {
		observer.Fields = f.saveResourceFields
	}
	if f.DebugResourceValues {
		observer.Values = f.saveResourceValues
	}
	return observer
}

func (f *CLIFrontend) saveResourceFields(report resourcediag.Report) {
	report.CapturedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(report, "", "  ")
	path := filepath.Join(f.Profile.Session.StateDir, "resource-fields.json")
	if err == nil {
		err = securefile.Write(path, append(data, '\n'))
	}
	if err != nil {
		log.Println("Unable to save resource field diagnostic; VPN setup will continue")
	} else if f.Diagnostic != nil {
		fmt.Fprintf(f.Diagnostic, "Resource field diagnostic saved (values omitted): %s\n", terminal.SafeText(path))
	}
}

func (f *CLIFrontend) saveResourceValues(snapshot resourcediag.ValueSnapshot) {
	// Raw values and parser/filesystem errors never enter status or log output.
	if snapshot.Err != nil || (snapshot.Format != "json" && snapshot.Format != "xml") {
		log.Println("Unable to capture resource values; VPN setup will continue")
		return
	}
	path := filepath.Join(f.Profile.Session.StateDir, "resource-values."+snapshot.Format)
	if err := securefile.Write(path, snapshot.Data); err != nil {
		log.Println("Unable to save resource values; VPN setup will continue")
	} else if f.Diagnostic != nil {
		fmt.Fprintf(f.Diagnostic, "Resource values saved (contains actual resource data; review before sharing): %s\n", terminal.SafeText(path))
	}
}
