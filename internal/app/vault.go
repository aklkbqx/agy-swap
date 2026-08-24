package app

import (
	"context"
	"fmt"
	"strings"
)

// AccountVault stores long-lived account tokens outside accounts.json. The
// file keeps only metadata and a stable reference, so backups and diagnostics
// do not accidentally expose bearer credentials.
type AccountVault interface {
	Get(context.Context, string) (string, bool)
	Set(context.Context, string, string) bool
	Delete(context.Context, string) bool
}

type osAccountVault struct{}

func NewAccountVault() AccountVault { return osAccountVault{} }

func (osAccountVault) Get(ctx context.Context, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	value := platformVaultGet(ctx, ref)
	return value, value != ""
}

func (osAccountVault) Set(ctx context.Context, ref, token string) bool {
	return strings.TrimSpace(ref) != "" && token != "" && platformVaultSet(ctx, ref, token)
}

func (osAccountVault) Delete(ctx context.Context, ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return true
	}
	return platformVaultDelete(ctx, ref)
}

func accountSecretRef(email string) string {
	email = normalizeEmail(email)
	if email == "" {
		return ""
	}
	return "account:" + email
}

func accountToken(ctx context.Context, account Account, vault AccountVault) (string, error) {
	if ref := getString(account, "secret_ref"); ref != "" {
		if vault == nil {
			return "", fmt.Errorf("account secret vault is unavailable")
		}
		if token, ok := vault.Get(ctx, ref); ok {
			return token, nil
		}
		return "", fmt.Errorf("secret vault entry %q is unavailable", ref)
	}
	if token := getString(account, "token_data"); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("account has no saved token")
}

func (a *Application) accountToken(ctx context.Context, account Account) (string, error) {
	return accountToken(ctx, account, a.vault)
}

func (a *Application) saveAccountSecret(ctx context.Context, account Account, token string) bool {
	email := getString(account, "email")
	ref := firstString(getString(account, "secret_ref"), accountSecretRef(email))
	if ref == "" || a.vault == nil || !a.vault.Set(ctx, ref, token) {
		return false
	}
	account["secret_ref"] = ref
	delete(account, "token_data")
	return true
}
