//go:build windows

package app

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func vaultTarget(ref string) (*uint16, bool) {
	target, err := windows.UTF16PtrFromString("agy-swap:" + ref)
	return target, err == nil
}

func platformVaultGet(_ context.Context, ref string) string {
	target, ok := vaultTarget(ref)
	if !ok {
		return ""
	}
	var pointer *credential
	okFlag, _, _ := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&pointer)))
	if okFlag == 0 || pointer == nil {
		return ""
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pointer)))
	if pointer.CredentialBlobSize == 0 {
		return ""
	}
	blob := unsafe.Slice(pointer.CredentialBlob, pointer.CredentialBlobSize)
	return string(blob)
}

func platformVaultSet(_ context.Context, ref, value string) bool {
	target, ok := vaultTarget(ref)
	if !ok || value == "" {
		return false
	}
	user, _ := windows.UTF16PtrFromString(ref)
	blob := []byte(value)
	item := credential{Type: credTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0], Persist: 3, UserName: user}
	okFlag, _, _ := procCredWrite.Call(uintptr(unsafe.Pointer(&item)), 0)
	return okFlag != 0
}

func platformVaultDelete(_ context.Context, ref string) bool {
	target, ok := vaultTarget(ref)
	if !ok {
		return false
	}
	okFlag, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	return okFlag != 0 || err == syscall.ERROR_NOT_FOUND
}
