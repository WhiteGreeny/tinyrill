package credentials

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

type systemBackend struct{}

var credentialDLL = windows.NewLazySystemDLL("advapi32.dll")
var credRead = credentialDLL.NewProc("CredReadW")
var credWrite = credentialDLL.NewProc("CredWriteW")
var credDelete = credentialDLL.NewProc("CredDeleteW")
var credFree = credentialDLL.NewProc("CredFree")

type credential struct {
	Flags, Type             uint32
	Target, Comment         *uint16
	LastWritten             windows.Filetime
	BlobSize                uint32
	Blob                    *byte
	Persist, AttributeCount uint32
	Attributes              unsafe.Pointer
	TargetAlias, Username   *uint16
}

func credentialError(op string, err error) error {
	if errors.Is(err, windows.ERROR_NOT_FOUND) {
		return ErrNotFound
	}
	if errors.Is(err, windows.ERROR_NO_SUCH_LOGON_SESSION) {
		return nativeError(op, ErrUnavailable)
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return nativeError(op, ErrLocked)
	}
	return nativeError(op, err)
}
func (systemBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return nil, err
	}
	var c *credential
	ok, _, err := credRead.Call(uintptr(unsafe.Pointer(name)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok == 0 {
		return nil, credentialError("read", err)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c == nil || c.BlobSize > maxRecord || (c.BlobSize > 0 && c.Blob == nil) {
		return nil, errors.New("invalid Windows credential blob")
	}
	return append([]byte(nil), unsafe.Slice(c.Blob, int(c.BlobSize))...), nil
}
func (systemBackend) Set(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 2560 {
		return errors.New("credential record exceeds Windows Credential Manager 2560-byte limit")
	}
	name, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return err
	}
	c := credential{Type: 1, Target: name, BlobSize: uint32(len(data)), Blob: &data[0], Persist: 2}
	ok, _, err := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(data)
	if ok == 0 {
		return credentialError("write", err)
	}
	return nil
}
func (systemBackend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(key)
	if err != nil {
		return err
	}
	ok, _, err := credDelete.Call(uintptr(unsafe.Pointer(name)), 1, 0)
	if ok == 0 {
		return credentialError("delete", err)
	}
	return nil
}
