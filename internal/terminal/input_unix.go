//go:build !windows

package terminal

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"io"
	"os"
	"unicode/utf8"
)

func readConsole(ctx context.Context, input *os.File, output io.Writer, hidden bool) (string, error) {
	fd := int(input.Fd())
	previous, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, previous)
	var data []byte
	defer func() { clear(data) }()
	var one [1]byte
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return "", io.EOF
		}
		n, err = unix.Read(fd, one[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", io.EOF
		}
		switch one[0] {
		case 3:
			return "", ErrCanceled
		case 4:
			return "", io.EOF
		case '\r', '\n':
			return string(data), nil
		case 8, 127:
			if len(data) > 0 {
				_, size := utf8.DecodeLastRune(data)
				data = data[:len(data)-size]
				if !hidden {
					fmt.Fprint(output, "\b \b")
				}
			}
		default:
			if one[0] < 32 {
				continue
			}
			if len(data) >= MaxInput {
				return "", errors.New("terminal input exceeds size limit")
			}
			data = append(data, one[0])
			if !hidden {
				output.Write(one[:])
			}
		}
	}
}
