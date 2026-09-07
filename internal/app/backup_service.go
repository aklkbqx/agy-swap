package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const maxBackupBytes = 64 * 1024 * 1024

type backupContents struct {
	accounts *Accounts
	settings *AppSettings
}

// readBackup is shared by verify and both import interfaces. No state is changed
// until every section, including configuration and credentials, has validated.
func readBackup(path, passphrase string) (*backupContents, error) {
	raw, err := readLimited(path, maxBackupBytes)
	if err != nil {
		return nil, err
	}
	var envelope encryptedBackup
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid backup: %w", err)
	}
	if envelope.Encrypted {
		raw, err = decryptBackup(passphrase, envelope)
		if err != nil {
			return nil, err
		}
	}
	var document struct {
		Schema   int             `json:"schema"`
		Accounts json.RawMessage `json:"accounts"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("invalid backup document: %w", err)
	}
	if document.Schema != stateSchema {
		return nil, fmt.Errorf("unsupported backup document schema %d", document.Schema)
	}
	accounts, err := decodeOrderedAccounts(document.Accounts)
	if err != nil {
		return nil, fmt.Errorf("invalid backup accounts: %w", err)
	}
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		// Vault references are machine-local and are never trusted from a file.
		delete(account, "secret_ref")
		if token := getString(account, "token_data"); token != "" {
			if !envelope.Encrypted {
				return nil, errors.New("secret backups must be encrypted")
			}
			if getString(tokenObject(decodeToken(token)), "access_token") == "" {
				return nil, fmt.Errorf("invalid backup credential for %s", email)
			}
		}
	}
	contents := &backupContents{accounts: accounts}
	if len(document.Settings) > 0 {
		var settings AppSettings
		if string(document.Settings) == "null" {
			return nil, errors.New("backup settings must be an object")
		}
		if err := json.Unmarshal(document.Settings, &settings); err != nil {
			return nil, err
		}
		settings, err = normalizeSettings(settings)
		if err != nil {
			return nil, err
		}
		contents.settings = &settings
	}
	return contents, nil
}

type restoreJournal struct {
	Schema    int    `json:"schema"`
	Committed bool   `json:"committed"`
	Accounts  []byte `json:"accounts"`
	Backup    []byte `json:"backup"`
	Settings  []byte `json:"settings"`
}

func (s *Store) restoreJournalPath() string {
	return filepath.Join(s.paths.ConfigDir, "restore-journal.json")
}

func (s *Store) restoreSnapshot(j restoreJournal) bool {
	return restoreFiles(fileSnapshot{s.paths.Accounts: j.Accounts, s.paths.AccountsBackup: j.Backup, s.paths.Settings: j.Settings})
}

// Recovery uses fixed local paths rather than paths supplied by the journal.
func (s *Store) recoverRestore() error {
	if _, err := os.Stat(s.restoreJournalPath()); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	accountsLock, err := acquireFileLock(s.paths.AccountsLock)
	if err != nil {
		return err
	}
	defer accountsLock.Close()
	settingsLock, err := acquireFileLock(s.paths.Settings + ".lock")
	if err != nil {
		return err
	}
	defer settingsLock.Close()
	return s.recoverRestoreUnlocked()
}

func (s *Store) recoverRestoreUnlocked() error {
	raw, err := readLimited(s.restoreJournalPath(), 4*maxBackupBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal restoreJournal
	if json.Unmarshal(raw, &journal) != nil || journal.Schema != 1 {
		return errors.New("invalid restore journal; recover from backup before continuing")
	}
	if !journal.Committed && !s.restoreSnapshot(journal) {
		return errors.New("cannot recover interrupted restore; journal retained")
	}
	return os.Remove(s.restoreJournalPath())
}

func (a *Application) importBackup(ctx context.Context, path, passphrase string, merge bool) (int, int, error) {
	contents, err := readBackup(path, passphrase)
	if err != nil {
		return 0, 0, err
	}
	s := a.store
	accountsLock, err := acquireFileLock(s.paths.AccountsLock)
	if err != nil {
		return 0, 0, err
	}
	defer accountsLock.Close()
	settingsLock, err := acquireFileLock(s.paths.Settings + ".lock")
	if err != nil {
		return 0, 0, err
	}
	defer settingsLock.Close()
	if err := s.recoverRestoreUnlocked(); err != nil {
		return 0, 0, err
	}
	existing, err := s.readAccountsUnlocked()
	if err != nil {
		return 0, 0, err
	}
	incoming := contents.accounts
	if merge {
		for _, email := range incoming.Order {
			account := incoming.ByEmail[email]
			if getString(account, "token_data") == "" {
				for _, key := range []string{"token_data", "secret_ref"} {
					if value, ok := existing.ByEmail[email][key]; ok {
						account[key] = value
					}
				}
			}
			existing.Set(email, account)
		}
		incoming = existing
	} else {
		incoming.Revision, incoming.revisionHash = existing.Revision, existing.revisionHash
	}
	if err := validateAccounts(incoming); err != nil {
		return 0, 0, err
	}
	snapshot, err := snapshotFiles(s.paths.Accounts, s.paths.AccountsBackup, s.paths.Settings)
	if err != nil {
		return 0, 0, err
	}
	journal := restoreJournal{Schema: 1, Accounts: snapshot[s.paths.Accounts], Backup: snapshot[s.paths.AccountsBackup], Settings: snapshot[s.paths.Settings]}
	if err := atomicWriteJSON(s.restoreJournalPath(), journal); err != nil {
		return 0, 0, err
	}
	var staged []string
	rollback := func(cause error) (int, int, error) {
		for _, ref := range staged {
			_ = a.vault.Delete(context.WithoutCancel(ctx), ref)
		}
		if !s.restoreSnapshot(journal) {
			return 0, 0, fmt.Errorf("%w; rollback incomplete, journal retained", cause)
		}
		if err := os.Remove(s.restoreJournalPath()); err != nil {
			return 0, 0, fmt.Errorf("%w; rollback journal cleanup: %v", cause, err)
		}
		return 0, 0, cause
	}
	for _, email := range incoming.Order {
		account := incoming.ByEmail[email]
		token := getString(account, "token_data")
		if token == "" {
			continue
		}
		if a.vault == nil {
			fmt.Fprintln(a.Err, "Warning: OS vault unavailable; imported credentials remain in local plaintext with restricted file permissions.")
			continue
		}
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		// Stage a new entry. Never overwrite a credential referenced by the
		// pre-restore metadata, so rollback remains safe even after a crash.
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return rollback(err)
		}
		ref := accountSecretRef(email) + ":" + hex.EncodeToString(suffix[:])
		if a.vault.Set(ctx, ref, token) {
			staged = append(staged, ref)
			account["secret_ref"] = ref
			delete(account, "token_data")
		} else {
			fmt.Fprintln(a.Err, "Warning: OS vault write failed; imported credentials remain in local plaintext with restricted file permissions.")
		}
	}
	if err := s.saveUnlocked(incoming); err != nil {
		return rollback(err)
	}
	if contents.settings != nil {
		settings := *contents.settings
		if merge {
			current, err := s.readSettingsUnlocked()
			if err != nil {
				return rollback(err)
			}
			// Merge collection metadata, while explicit imported preferences win.
			for k, v := range current.Aliases {
				if _, ok := settings.Aliases[k]; !ok {
					settings.Aliases[k] = v
				}
			}
			for k, v := range current.Tags {
				if _, ok := settings.Tags[k]; !ok {
					settings.Tags[k] = v
				}
			}
			for k, v := range current.Profiles {
				if _, ok := settings.Profiles[k]; !ok {
					settings.Profiles[k] = v
				}
			}
			for k, v := range current.Targets {
				if _, ok := settings.Targets[k]; !ok {
					settings.Targets[k] = v
				}
			}
			for _, b := range current.Bindings {
				found := false
				for _, n := range settings.Bindings {
					if b.Path == n.Path {
						found = true
						break
					}
				}
				if !found {
					settings.Bindings = append(settings.Bindings, b)
				}
			}
		}
		if err := s.saveSettingsUnlocked(settings); err != nil {
			return rollback(err)
		}
	}
	journal.Committed = true
	if err := atomicWriteJSON(s.restoreJournalPath(), journal); err != nil {
		return rollback(err)
	}
	if err := os.Remove(s.restoreJournalPath()); err != nil {
		return incoming.Len(), len(staged), fmt.Errorf("restore committed; journal cleanup failed: %w", err)
	}
	return incoming.Len(), len(staged), nil
}
