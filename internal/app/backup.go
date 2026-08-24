package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
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
	if len(passphrase) < 8 {
		return encryptedBackup{}, errors.New("passphrase must contain at least 8 characters")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return encryptedBackup{}, err
	}
	block, err := aes.NewCipher(backupKey(passphrase, salt))
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
	return encryptedBackup{Schema: stateSchema, Encrypted: true, KDF: "sha256-120k", Salt: base64.StdEncoding.EncodeToString(salt), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}, nil
}

func decryptBackup(passphrase string, envelope encryptedBackup) ([]byte, error) {
	if !envelope.Encrypted {
		return nil, errors.New("backup is not encrypted")
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.Salt)
	if err != nil {
		return nil, errors.New("invalid backup salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, errors.New("invalid backup nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid backup ciphertext")
	}
	block, err := aes.NewCipher(backupKey(passphrase, salt))
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
	return strings.TrimSpace(string(data)), nil
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
		raw, err := os.ReadFile(positional[1])
		if err != nil {
			return a.extendedError("backup import", opts, err)
		}
		var envelope encryptedBackup
		var object map[string]any
		if json.Unmarshal(raw, &envelope) == nil && envelope.Encrypted {
			plaintext, decryptErr := decryptBackup(opts.Passphrase, envelope)
			if decryptErr != nil {
				return a.extendedError("backup import", opts, decryptErr)
			}
			if err := json.Unmarshal(plaintext, &object); err != nil {
				return a.extendedError("backup import", opts, errors.New("decrypted backup is invalid"))
			}
		} else if err := json.Unmarshal(raw, &object); err != nil {
			return a.extendedError("backup import", opts, fmt.Errorf("invalid backup: %w", err))
		}
		if _, ok := object["accounts"]; !ok {
			return a.extendedError("backup import", opts, errors.New("backup has no accounts"))
		}
		accountData, err := json.Marshal(object["accounts"])
		if err != nil {
			return a.extendedError("backup import", opts, err)
		}
		incoming, err := decodeOrderedAccounts(accountData)
		if err != nil {
			return a.extendedError("backup import", opts, err)
		}
		if opts.Merge {
			existing, loadErr := a.store.Load(false)
			if loadErr != nil {
				return a.extendedError("backup import", opts, loadErr)
			}
			for _, email := range incoming.Order {
				existing.Set(email, incoming.ByEmail[email])
			}
			incoming = existing
		}
		migrated := 0
		for _, email := range incoming.Order {
			account := incoming.ByEmail[email]
			if token := getString(account, "token_data"); token != "" && a.saveAccountSecret(ctx, account, token) {
				migrated++
			}
		}
		if err := a.store.Save(incoming); err != nil {
			return a.extendedError("backup import", opts, err)
		}
		if rawSettings, ok := object["settings"]; ok {
			var settings AppSettings
			if json.Unmarshal(mustJSON(rawSettings), &settings) == nil {
				_ = a.store.SaveSettings(settings)
			}
		}
		data := map[string]any{"accounts": incoming.Len(), "vault_migrated": migrated, "merged": opts.Merge}
		if opts.JSON {
			return a.extendedResult("backup import", opts, data, nil)
		}
		fmt.Fprintf(a.Out, "Imported %d account(s); migrated %d secret(s) to the OS vault.\n", incoming.Len(), migrated)
		return 0
	case "verify":
		if len(positional) < 2 {
			return a.extendedError("backup verify", opts, errors.New("usage: backup verify FILE [--passphrase PASS]"))
		}
		raw, err := os.ReadFile(positional[1])
		if err != nil {
			return a.extendedError("backup verify", opts, err)
		}
		var envelope encryptedBackup
		if json.Unmarshal(raw, &envelope) == nil && envelope.Encrypted {
			_, err = decryptBackup(opts.Passphrase, envelope)
		} else {
			var object map[string]any
			err = json.Unmarshal(raw, &object)
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
