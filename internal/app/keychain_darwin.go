//go:build darwin && cgo

package app

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation -framework LocalAuthentication
#include <stdlib.h>
int storeSecret(const char *service, const char *account, const void *secret, long size);
*/
import "C"

import (
	"context"
	"strings"
	"unsafe"
)

// Send secret bytes directly to Security.framework, never through process args.
func keychainSet(ctx context.Context, service, account, token string) bool {
	if ctx.Err() != nil || token == "" || strings.ContainsRune(service, 0) || strings.ContainsRune(account, 0) {
		return false
	}
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	data := []byte(token)
	return C.storeSecret(s, a, unsafe.Pointer(&data[0]), C.long(len(data))) != 0
}
