package terminal

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"testing"
	"time"
)

func TestNativePTYHiddenInputAndCancellation(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	original, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		value string
		err   error
	}
	for _, cancelInput := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second*3)
		done := make(chan result, 1)
		go func() { value, err := readConsole(ctx, slave, io.Discard, true); done <- result{value, err} }()
		deadline := time.Now().Add(time.Second)
		for {
			state, _ := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if state.Lflag&unix.ECHO == 0 {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				t.Fatal("console did not enter hidden mode")
			}
			time.Sleep(time.Millisecond)
		}
		if cancelInput {
			cancel()
		} else {
			_, err = master.Write([]byte("synthetic-密码\r"))
			if err != nil {
				t.Fatal(err)
			}
		}
		got := <-done
		cancel()
		if cancelInput {
			if !errors.Is(got.err, context.Canceled) {
				t.Fatal(got.err)
			}
		} else if got.err != nil || got.value != "synthetic-密码" {
			t.Fatalf("input result=%q %v", got.value, got.err)
		}
		restored, _ := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
		if *restored != *original {
			t.Fatal("terminal mode was not restored")
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if n, _ := unix.Poll(poll, 10); n != 0 {
			t.Fatal("hidden input was echoed")
		}
	}
}
