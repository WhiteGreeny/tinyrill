// Package securefile protects application-owned secret files using native
// permissions. It never follows a final-component symlink when creating files.
package securefile

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Create(path string) (*os.File, error) {
	return openPrivate(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
}

// OpenLock preserves the inode and contents of a process lock file.
func OpenLock(path string) (*os.File, error) {
	return openPrivate(path, os.O_CREATE|os.O_RDWR)
}

// Write atomically replaces a file after writing and syncing a private sibling.
func Write(path string, data []byte) error {
	temporary := filepath.Join(filepath.Dir(path), ".tinyrill-"+rand.Text())
	f, err := openPrivate(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncParent(filepath.Dir(path))
}

// ReadLimited rejects non-regular files and oversized input before using it.
// External read-only secret mounts can have deployment-managed permissions.
func ReadLimited(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("secret must be a regular file within the size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("secret file changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("secret exceeds size limit")
	}
	return data, nil
}
