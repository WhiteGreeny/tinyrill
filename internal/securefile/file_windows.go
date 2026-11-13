package securefile

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func privateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
}

func privateHandle(path string, flags int, directory bool) (windows.Handle, error) {
	sd, err := privateDescriptor()
	if err != nil {
		return 0, err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	disposition := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		disposition = windows.OPEN_ALWAYS
	}
	if flags&os.O_EXCL != 0 {
		disposition = windows.CREATE_NEW
	}
	attributes := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	access := uint32(windows.GENERIC_READ | windows.GENERIC_WRITE | windows.WRITE_DAC | windows.READ_CONTROL)
	if directory {
		attributes |= windows.FILE_FLAG_BACKUP_SEMANTICS
		access = windows.WRITE_DAC | windows.READ_CONTROL
	}
	h, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, sa, disposition, attributes, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err == nil && (info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (!directory && info.NumberOfLinks != 1)) {
		err = errors.New("private path must not be a reparse point or hard link")
	}
	if err == nil {
		var acl *windows.ACL
		acl, _, err = sd.DACL()
		if err == nil {
			err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
		}
	}
	if err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

func openPrivate(path string, flags int) (*os.File, error) {
	h, err := privateHandle(path, flags, false)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if flags&os.O_TRUNC != 0 {
		if err := f.Truncate(0); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func MakeDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	h, err := privateHandle(path, 0, true)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}

// Check verifies the actual protected DACL, not Windows' emulated Unix mode.
func Check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("not a regular private path")
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 || acl == nil || acl.AceCount != 1 {
		return errors.New("private DACL is not restricted to one principal")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !windows.EqualSid(sid, user.User.Sid) || uint32(ace.Mask)&fileAllAccess != fileAllAccess {
		return errors.New("private DACL does not grant only this user full control")
	}
	return nil
}

func syncParent(string) error { return nil }
