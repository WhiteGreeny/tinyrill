//go:build darwin && cgo

package credentials

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef query(const char *key) {
    CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
    CFStringRef account = CFStringCreateWithCString(NULL,key,kCFStringEncodingUTF8);
    CFDictionarySetValue(q,kSecClass,kSecClassGenericPassword);
    CFDictionarySetValue(q,kSecAttrService,CFSTR("tinyrill"));
    CFDictionarySetValue(q,kSecAttrAccount,account);
    CFRelease(account);
    return q;
}
static OSStatus readSecret(const char *key, void **bytes, int *length) {
    CFMutableDictionaryRef q = query(key);
    CFDictionarySetValue(q,kSecReturnData,kCFBooleanTrue);
    CFDictionarySetValue(q,kSecMatchLimit,kSecMatchLimitOne);
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching(q,&result);
    CFRelease(q);
    if (status != errSecSuccess) return status;
    if (!result || CFGetTypeID(result)!=CFDataGetTypeID() || CFDataGetLength((CFDataRef)result)>32768) {
        if (result) CFRelease(result);
        return errSecDecode;
    }
    *length = (int)CFDataGetLength((CFDataRef)result);
    *bytes = malloc(*length ? *length : 1);
    if (!*bytes) { CFRelease(result); return errSecAllocate; }
    memcpy(*bytes,CFDataGetBytePtr((CFDataRef)result),*length);
    CFRelease(result);
    return errSecSuccess;
}
static OSStatus writeSecret(const char *key, const void *bytes, int length) {
    CFMutableDictionaryRef q = query(key);
    CFMutableDictionaryRef updates = CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
    CFDataRef data = CFDataCreate(NULL,(const UInt8*)bytes,length);
    CFDictionarySetValue(updates,kSecValueData,data);
    OSStatus status = SecItemUpdate(q,updates);
    if (status == errSecItemNotFound) {
        CFDictionarySetValue(q,kSecValueData,data);
        status = SecItemAdd(q,NULL);
    }
    CFRelease(data); CFRelease(updates); CFRelease(q);
    return status;
}
static OSStatus deleteSecret(const char *key) {
    CFMutableDictionaryRef q = query(key);
    OSStatus status = SecItemDelete(q);
    CFRelease(q);
    return status;
}
*/
import "C"
import (
	"context"
	"fmt"
	"unsafe"
)

type systemBackend struct{}

func keychainError(code C.OSStatus) error {
	if code == C.errSecSuccess {
		return nil
	}
	if code == C.errSecItemNotFound {
		return ErrNotFound
	}
	if code == C.errSecAuthFailed || code == C.errSecInteractionNotAllowed || code == C.errSecUserCanceled {
		return ErrLocked
	}
	return fmt.Errorf("Keychain error %d: %w", int(code), ErrUnavailable)
}
func (systemBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	var data unsafe.Pointer
	var size C.int
	if err := keychainError(C.readSecret(k, &data, &size)); err != nil {
		return nil, err
	}
	defer C.free(data)
	return C.GoBytes(data, size), nil
}
func (systemBackend) Set(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	return keychainError(C.writeSecret(k, unsafe.Pointer(unsafe.SliceData(data)), C.int(len(data))))
}
func (systemBackend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	return keychainError(C.deleteSecret(k))
}
