// Package log emits bounded diagnostic metadata. Protocol payloads never enter logs.
package log

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
)

var debug atomic.Bool
var secretField = regexp.MustCompile(`(?i)(password|passwd|cookie|authorization|sid|twfid|token|signkey|csrf|ticket|secret)["']?\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
var urlValue = regexp.MustCompile(`https?://[^\s<>"']+`)

// Redact is a second line of defence; callers must log metadata, never payloads.
func Redact(value string) string {
	if len(value) > 8192 {
		value = value[:8192] + " [truncated]"
	}
	value = secretField.ReplaceAllString(value, "$1=[redacted]")
	value = urlValue.ReplaceAllStringFunc(value, func(s string) string {
		start := strings.Index(s, "://") + 3
		if at := strings.Index(s[start:], "@"); at >= 0 {
			s = s[:start] + "[redacted]@" + s[start+at+1:]
		}
		if at := strings.IndexAny(s, "?#"); at >= 0 {
			s = s[:at] + "?[redacted]"
		}
		return s
	})
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f) {
			return ' '
		}
		return r
	}, value)
}
func Init()                          { log.SetOutput(os.Stderr) }
func EnableDebug()                   { debug.Store(true) }
func DisableDebug()                  { debug.Store(false) }
func DebugEnabled() bool             { return debug.Load() }
func Print(v ...any)                 { log.Print(Redact(fmt.Sprint(v...))) }
func Println(v ...any)               { log.Print(Redact(strings.TrimSuffix(fmt.Sprintln(v...), "\n"))) }
func Printf(format string, v ...any) { log.Print(Redact(fmt.Sprintf(format, v...))) }
func DebugPrint(v ...any) {
	if debug.Load() {
		Print(v...)
	}
}
func DebugPrintln(v ...any) {
	if debug.Load() {
		Println(v...)
	}
}
func DebugPrintf(format string, v ...any) {
	if debug.Load() {
		Printf(format, v...)
	}
}

// Kept for protocol instrumentation: only the byte count is emitted.
func DumpHex(buf []byte) { Printf("Payload omitted (%d bytes)", len(buf)) }
func DebugDumpHex(buf []byte) {
	if debug.Load() {
		DumpHex(buf)
	}
}

type redactingWriter struct{}

func (redactingWriter) Write(p []byte) (int, error) {
	_, err := fmt.Fprintln(os.Stderr, Redact(string(p)))
	return len(p), err
}
func NewLogger(prefix string) *log.Logger { return log.New(redactingWriter{}, prefix, log.LstdFlags) }
