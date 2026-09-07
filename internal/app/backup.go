package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type encryptedBackup struct {
	Schema     int    `json:"schema"`
	Encrypted  bool   `json:"encrypted"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func backupKey(passphrase string, salt []byte) []byte {
	seed := append([]byte(passphrase), salt...)
	digest := sha256.Sum256(seed)
	key := digest[:]
	for i := 0; i < 120000; i++ {
		next := sha256.Sum256(append(key, seed...))
		key = next[:]
	}
	return key
}

func encryptBackup(passphrase string, plaintext []byte) (encryptedBackup, error) {
	if utf8.RuneCountInString(passphrase) < 8 {
		return encryptedBackup{}, errors.New("passphrase must contain at least 8 characters")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return encryptedBackup{}, err
	}
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	if err != nil {
		return encryptedBackup{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedBackup{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedBackup{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedBackup{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return encryptedBackup{Schema: 2, Encrypted: true, KDF: "pbkdf2-sha256-600k", Salt: base64.StdEncoding.EncodeToString(salt), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}, nil
}

func decryptBackup(passphrase string, envelope encryptedBackup) ([]byte, error) {
	if !envelope.Encrypted {
		return nil, errors.New("backup is not encrypted")
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.Salt)
	if err != nil || len(salt) != 16 {
		return nil, errors.New("invalid backup salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != 12 {
		return nil, errors.New("invalid backup nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid backup ciphertext")
	}
	var key []byte
	switch {
	case envelope.Schema == 1 && envelope.KDF == "sha256-120k":
		key = backupKey(passphrase, salt)
	case envelope.Schema == 2 && envelope.KDF == "pbkdf2-sha256-600k":
		key, err = pbkdf2.Key(sha256.New, passphrase, salt, 600000, 32)
	default:
		return nil, errors.New("unsupported backup schema or KDF")
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("wrong passphrase or corrupted backup")
	}
	return plaintext, nil
}

func (a *Application) backupDocument(ctx context.Context, includeSecrets bool) ([]byte, error) {
	accountsData, err := os.ReadFile(a.paths.Accounts)
	if errors.Is(err, os.ErrNotExist) {
		accountsData = []byte("{}")
	} else if err != nil {
		return nil, err
	}
	accounts, decodeErr := decodeOrderedAccounts(accountsData)
	if decodeErr != nil {
		return nil, decodeErr
	}
	for _, email := range accounts.Order {
		if includeSecrets {
			if token, tokenErr := a.accountToken(ctx, accounts.ByEmail[email]); tokenErr == nil {
				accounts.ByEmail[email]["token_data"] = token
			} else {
				return nil, fmt.Errorf("cannot export secret for %s: %w", email, tokenErr)
			}
		} else {
			delete(accounts.ByEmail[email], "token_data")
		}
		delete(accounts.ByEmail[email], "secret_ref")
	}
	accountsData, err = encodeOrderedAccounts(accounts)
	if err != nil {
		return nil, err
	}
	settings, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	document := map[string]any{"schema": stateSchema, "created_at": isoTime(time.Now().UTC()), "version": a.Version, "settings": settings, "accounts": json.RawMessage(accountsData), "contains_secrets": includeSecrets}
	return json.MarshalIndent(document, "", "  ")
}

func (a *Application) backupPassphrase(opts extendedOptions) (string, error) {
	if !opts.PassphraseStdin {
		return opts.Passphrase, nil
	}
	data, err := io.ReadAll(io.LimitReader(a.In, maxTokenBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxTokenBytes {
		return "", errors.New("passphrase input exceeds limit")
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}

func (a *Application) cmdBackup(ctx context.Context, opts extendedOptions, positional []string) int {
	sub := "export"
	if len(positional) > 0 {
		sub = positional[0]
	}
	switch sub {
	case "export":
		passphrase, passphraseErr := a.backupPassphrase(opts)
		if passphraseErr != nil {
			return a.extendedError("backup export", opts, passphraseErr)
		}
		if opts.IncludeSecrets {
			opts.Passphrase = passphrase
		}
		plaintext, err := a.backupDocument(ctx, opts.IncludeSecrets)
		if err != nil {
			return a.extendedError("backup export", opts, err)
		}
		var output []byte
		encrypted := false
		if opts.IncludeSecrets {
			envelope, encryptErr := encryptBackup(opts.Passphrase, plaintext)
			if encryptErr != nil {
				return a.extendedError("backup export", opts, encryptErr)
			}
			output, err = json.MarshalIndent(envelope, "", "  ")
			encrypted = true
		} else {
			output = plaintext
		}
		target := opts.Output
		if target == "" && len(positional) > 1 {
			target = positional[1]
		}
		if target == "" {
			target = "agy-swap-backup.json"
		}
		if target == "-" {
			fmt.Fprintln(a.Out, string(output))
			return 0
		}
		if err := atomicWrite(target, append(output, '\n'), 0o600); err != nil {
			return a.extendedError("backup export", opts, err)
		}
		data := map[string]any{"path": target, "encrypted": encrypted, "contains_secrets": opts.IncludeSecrets}
		if opts.JSON {
			return a.extendedResult("backup export", opts, data, nil)
		}
		fmt.Fprintf(a.Out, "Backup written to %s (%s).\n", target, map[bool]string{true: "encrypted", false: "metadata-only"}[encrypted])
		return 0
	case "import":
		passphrase, passphraseErr := a.backupPassphrase(opts)
		if passphraseErr != nil {
			return a.extendedError("backup import", opts, passphraseErr)
		}
		if passphrase != "" {
			opts.Passphrase = passphrase
		}
		if len(positional) < 2 {
			return a.extendedError("backup import", opts, errors.New("usage: backup import FILE [--merge] [--passphrase PASS]"))
		}
		count, migrated, err := a.importBackup(ctx, positional[1], opts.Passphrase, opts.Merge)
		if err != nil {
			return a.extendedError("backup import", opts, err)
		}

		data := map[string]any{"accounts": count, "vault_migrated": migrated, "merged": opts.Merge}
		if opts.JSON {
			return a.extendedResult("backup import", opts, data, nil)
		}
		fmt.Fprintf(a.Out, "Imported %d account(s); migrated %d secret(s) to the OS vault.\n", count, migrated)
		return 0
	case "verify":
		if len(positional) < 2 {
			return a.extendedError("backup verify", opts, errors.New("usage: backup verify FILE [--passphrase PASS]"))
		}
		passphrase, err := a.backupPassphrase(opts)
		if err == nil {
			_, err = readBackup(positional[1], passphrase)
		}
		if err != nil {
			return a.extendedError("backup verify", opts, err)
		}

		if opts.JSON {
			return a.extendedResult("backup verify", opts, map[string]any{"valid": true}, nil)
		}
		fmt.Fprintln(a.Out, "Backup is valid.")
		return 0
	default:
		return a.extendedError("backup", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }
