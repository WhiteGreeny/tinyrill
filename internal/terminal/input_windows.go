package terminal

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"unicode/utf16"
	"unsafe"
)

var readConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

type keyEvent struct {
	Type                           uint16
	_                              uint16
	Down                           int32
	Repeat, VirtualKey, Scan, Char uint16
	Control                        uint32
}

func readConsole(ctx context.Context, input *os.File, output io.Writer, hidden bool) (string, error) {
	h := windows.Handle(input.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return "", err
	}
	// Ctrl+C is handled as input; cancellation always restores the console.
	if err := windows.SetConsoleMode(h, mode&^(windows.ENABLE_ECHO_INPUT|windows.ENABLE_LINE_INPUT|windows.ENABLE_PROCESSED_INPUT|windows.ENABLE_VIRTUAL_TERMINAL_INPUT)); err != nil {
		return "", err
	}
	defer windows.SetConsoleMode(h, mode)
	var data []uint16
	defer func() { clear(data) }()
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		status, err := windows.WaitForSingleObject(h, 50)
		if err != nil {
			return "", err
		}
		if status == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		var event keyEvent
		var count uint32
		ok, _, callErr := readConsoleInput.Call(uintptr(h), uintptr(unsafe.Pointer(&event)), 1, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return "", callErr
		}
		if count != 1 || event.Type != 1 || event.Down == 0 {
			continue
		}
		for range event.Repeat {
			switch event.Char {
			case 3:
				return "", ErrCanceled
			case 4:
				return "", io.EOF
			case '\r':
				return string(utf16.Decode(data)), nil
			case 8:
				if len(data) > 0 {
					data = data[:len(data)-1]
					if !hidden {
						fmt.Fprint(output, "\b \b")
					}
				}
			default:
				if event.Char < 32 {
					continue
				}
				if len(data) >= MaxInput {
					return "", errors.New("terminal input exceeds size limit")
				}
				data = append(data, event.Char)
				if !hidden && !utf16.IsSurrogate(rune(event.Char)) {
					fmt.Fprint(output, string(rune(event.Char)))
				}
			}
		}
	}
}
