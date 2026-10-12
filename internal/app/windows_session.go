package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// CRED_MAX_CREDENTIAL_BLOB_SIZE, in bytes (wincred.h).
const windowsCredentialBlobLimit = 2560

var errWindowsCredentialTooLarge = errors.New("Windows credential exceeds the Credential Manager size limit") //nolint:staticcheck // "Windows" is a proper noun

// Windows consumes compact JSON rather than the saved vault's base64 wrapper.
// Refresh prefers a nested ID token; promote it before removing its duplicate.
// All other provider fields are retained, and oversized data fails before writes.
func windowsSessionCredential(value string) (string, error) {
	raw := []byte(value)
	if strings.HasPrefix(value, "go-keyring-base64:") {
		var err error
		raw, err = base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "go-keyring-base64:"))
		if err != nil {
			return "", errors.New("invalid session credential encoding")
		}
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&decoded) != nil || decoded == nil {
		return "", errors.New("invalid session credential JSON")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return "", errors.New("trailing session credential data")
	}
	if nested := tokenObject(decoded); nested != nil {
		if id := getString(nested, "id_token"); id != "" {
			decoded["id_token"] = id
			delete(nested, "id_token")
		}
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return "", errors.New("could not encode session credential")
	}
	if len(data) > windowsCredentialBlobLimit {
		return "", errWindowsCredentialTooLarge
	}
	return string(data), nil
}
