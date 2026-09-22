//go:build darwin && cgo

package app

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation -framework LocalAuthentication
#include <stdlib.h>
int storeSecret(const char *service, const char *account, const void *secret, long size);
int loadSecret(const char *service, const char *account, void **out, long *size);
int deleteSecret(const char *service, const char *account);
*/
import "C"

import (
	"context"
	"strings"
	"unsafe"
)

func keychainUsable(ctx context.Context, service, account string) bool {
	return ctx.Err() == nil && !strings.ContainsRune(service, 0) && !strings.ContainsRune(account, 0)
}

// Send vault secret bytes directly to Security.framework, never through process args.
// The gemini/antigravity session item is published by /usr/bin/security instead.
func keychainSet(ctx context.Context, service, account, token string) bool {
	if !keychainUsable(ctx, service, account) || token == "" {
		return false
	}
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	data := []byte(token)
	return C.storeSecret(s, a, unsafe.Pointer(&data[0]), C.long(len(data))) != 0
}

// Read through the same framework the write went through. This is the agy-swap
// vault (service agy-swap). The gemini/antigravity session item is owned by
// /usr/bin/security in credential_unix.go, because the agy CLI reads it by
// spawning that tool. A second code identity fails the item's partition list.
func keychainGet(ctx context.Context, service, account string) string {
	if !keychainUsable(ctx, service, account) {
		return ""
	}
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var buffer unsafe.Pointer
	var size C.long
	if C.loadSecret(s, a, &buffer, &size) == 0 {
		return ""
	}
	defer C.free(buffer)
	return strings.TrimSpace(string(C.GoBytes(buffer, C.int(size))))
}

func keychainDelete(ctx context.Context, service, account string) bool {
	if !keychainUsable(ctx, service, account) {
		return false
	}
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	return C.deleteSecret(s, a) != 0
}
