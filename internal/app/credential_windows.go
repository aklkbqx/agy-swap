//go:build windows

package app

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const credTypeGeneric = 1

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var advapi = windows.NewLazySystemDLL("advapi32.dll")
var procCredRead = advapi.NewProc("CredReadW")
var procCredWrite = advapi.NewProc("CredWriteW")
var procCredDelete = advapi.NewProc("CredDeleteW")
var procCredFree = advapi.NewProc("CredFree")

func platformCredentialGet(context.Context) string {
	target, _ := windows.UTF16PtrFromString("gemini:antigravity")
	var pointer *credential
	ok, _, _ := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&pointer)))
	if ok == 0 || pointer == nil {
		return ""
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pointer)))
	if pointer.CredentialBlobSize == 0 {
		return ""
	}
	blob := unsafe.Slice(pointer.CredentialBlob, pointer.CredentialBlobSize)
	return string(blob)
}
func platformCredentialSet(_ context.Context, value string) bool {
	target, _ := windows.UTF16PtrFromString("gemini:antigravity")
	user, _ := windows.UTF16PtrFromString("antigravity")
	blob := []byte(value)
	if len(blob) == 0 {
		return false
	}
	item := credential{Type: credTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0], Persist: 3, UserName: user}
	ok, _, _ := procCredWrite.Call(uintptr(unsafe.Pointer(&item)), 0)
	return ok != 0
}
func platformCredentialDelete(context.Context) bool {
	target, _ := windows.UTF16PtrFromString("gemini:antigravity")
	ok, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	return ok != 0 || err == syscall.ERROR_NOT_FOUND
}
