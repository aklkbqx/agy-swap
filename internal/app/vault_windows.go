//go:build windows

package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	winCredMaxBlobSize = 2560
	winVaultGzipPrefix = "agy-swap-gz:"
)

func encodeWindowsVaultBlob(value string) ([]byte, bool) {
	blob := []byte(value)
	if len(blob) <= winCredMaxBlobSize {
		return blob, true
	}
	var buf bytes.Buffer
	buf.WriteString(winVaultGzipPrefix)
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, false
	}
	if _, err := gz.Write(blob); err != nil {
		_ = gz.Close()
		return nil, false
	}
	if err := gz.Close(); err != nil {
		return nil, false
	}
	compressed := buf.Bytes()
	if len(compressed) > winCredMaxBlobSize {
		return nil, false
	}
	return compressed, true
}

func decodeWindowsVaultBlob(blob []byte) string {
	prefix := []byte(winVaultGzipPrefix)
	if !bytes.HasPrefix(blob, prefix) {
		return string(blob)
	}
	gz, err := gzip.NewReader(bytes.NewReader(blob[len(prefix):]))
	if err != nil {
		return ""
	}
	defer func() { _ = gz.Close() }()
	plain, err := io.ReadAll(io.LimitReader(gz, maxTokenBytes+1))
	if err != nil || len(plain) > maxTokenBytes {
		return ""
	}
	return string(plain)
}

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
	return decodeWindowsVaultBlob(blob)
}

func platformVaultSet(_ context.Context, ref, value string) bool {
	target, ok := vaultTarget(ref)
	if !ok || value == "" {
		return false
	}
	blob, fits := encodeWindowsVaultBlob(value)
	if !fits || len(blob) == 0 {
		return false
	}
	user, _ := windows.UTF16PtrFromString(ref)
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
	return okFlag != 0 || err == syscall.ERROR_NOT_FOUND || err == errorNoSuchLogonSession
}
