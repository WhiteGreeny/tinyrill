//go:build !windows

package securefile

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func openPrivate(path string, flags int) (*os.File, error) {
	fd, err := unix.Open(path, (flags&^os.O_TRUNC)|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	var info unix.Stat_t
	if err = unix.Fstat(fd, &info); err == nil && (info.Uid != uint32(os.Geteuid()) || info.Nlink != 1 || info.Mode&unix.S_IFMT != unix.S_IFREG) {
		err = errors.New("private file must be a regular file owned by this user with one link")
	}
	if err == nil {
		err = f.Chmod(0600)
	}
	if err == nil && flags&os.O_TRUNC != 0 {
		err = f.Truncate(0)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func MakeDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		return err
	}
	if info.Uid != uint32(os.Geteuid()) {
		return errors.New("state directory is not owned by this user")
	}
	return unix.Fchmod(fd, 0700)
}

func Check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	want := os.FileMode(0600)
	if info.IsDir() {
		want = 0700
	} else if !info.Mode().IsRegular() {
		return errors.New("not a regular private file")
	}
	if info.Mode().Perm() != want {
		return errors.New("private file permissions are too broad")
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return errors.New("private file is not owned by this user")
	}
	return nil
}

func syncParent(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
