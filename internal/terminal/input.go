// Package terminal provides bounded, cancellable foreground input. It does not
// read redirected stdin, spawn a background reader or put secrets in argv.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"strings"
	"sync"
	"unicode"
)

var ErrInteractionRequired = errors.New("foreground terminal interaction is required")
var ErrCanceled = errors.New("terminal input canceled by user")
var inputMu sync.Mutex

const MaxInput = 4096

func Available() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

func SafeText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, text)
}

func ReadLine(ctx context.Context, output io.Writer, prompt string, hidden bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !Available() {
		return "", ErrInteractionRequired
	}
	if !inputMu.TryLock() {
		return "", errors.New("another terminal prompt is pending")
	}
	defer inputMu.Unlock()
	if output == nil {
		output = os.Stderr
	}
	fmt.Fprint(output, SafeText(prompt)+" ")
	defer fmt.Fprint(output, "\r\n")
	return readConsole(ctx, os.Stdin, output, hidden)
}
