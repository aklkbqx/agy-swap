"""Account store CRUD, JSON schema validation, and legacy quota migration."""

from datetime import datetime, timezone
import json
import os
import re

import agy_swap
from agy_swap import ACCOUNTS_FILE, QUOTA_SCHEMA, MAX_LIMIT_SECS, Accounts, AccountStoreError
from agy_swap.display import clean_display_text, normalize_email, _parse_utc_datetime
from agy_swap.logs import auto_scan_logs_for_limits, _model_identity
from agy_swap.oauth import decode_token, extract_verified_google_email_claim
from agy_swap.storage import _atomic_write_bytes, _accounts_lock


def _normalize_quota_snapshot(snapshot, email):
    if not isinstance(snapshot, dict):
        raise AccountStoreError(f"Invalid quota snapshot for {email}")
    try:
        observed_at = _parse_utc_datetime(snapshot["observed_at"]).isoformat()
    except (KeyError, AttributeError, TypeError, ValueError):
        raise AccountStoreError(f"Invalid quota snapshot timestamp for {email}") from None

    tier = snapshot.get("tier")
    if not isinstance(tier, dict):
        raise AccountStoreError(f"Invalid quota tier for {email}")
    tier_id = clean_display_text(tier.get("id"))
    tier_name = clean_display_text(tier.get("name"))
    if not tier_id or not tier_name:
        raise AccountStoreError(f"Invalid quota tier for {email}")

    raw_groups = snapshot.get("groups")
    if not isinstance(raw_groups, list) or not raw_groups:
        raise AccountStoreError(f"Invalid quota groups for {email}")
    groups = []
    for group in raw_groups[:8]:
        if not isinstance(group, dict) or group.get("id") not in ("gemini", "third_party"):
            raise AccountStoreError(f"Invalid quota group for {email}")
        name = clean_display_text(group.get("name"))
        raw_buckets = group.get("buckets")
        if not name or not isinstance(raw_buckets, list) or not raw_buckets:
            raise AccountStoreError(f"Invalid quota group for {email}")
        buckets = []
        for bucket in raw_buckets[:8]:
            if not isinstance(bucket, dict) or bucket.get("window") not in ("weekly", "5h"):
                raise AccountStoreError(f"Invalid quota bucket for {email}")
            bucket_id = clean_display_text(bucket.get("id"))
            bucket_name = clean_display_text(bucket.get("name"))
            fraction = bucket.get("remaining_fraction")
            if not bucket_id or not bucket_name or isinstance(fraction, bool) or not isinstance(fraction, (int, float)) or not 0 <= fraction <= 1:
                raise AccountStoreError(f"Invalid quota bucket for {email}")
            try:
                reset_at = _parse_utc_datetime(bucket["reset_at"]).isoformat()
            except (KeyError, AttributeError, TypeError, ValueError):
                raise AccountStoreError(f"Invalid quota reset for {email}") from None
            buckets.append({
                "id": bucket_id,
                "name": bucket_name,
                "window": bucket["window"],
                "remaining_fraction": float(fraction),
                "reset_at": reset_at,
            })
        groups.append({"id": group["id"], "name": name, "buckets": buckets})

    return {
        "observed_at": observed_at,
        "tier": {"id": tier_id, "name": tier_name},
        "groups": groups,
    }


def _validate_accounts(data):
    if not isinstance(data, dict):
        raise AccountStoreError("accounts.json must contain an object")
    accounts = Accounts()
    for key, value in data.items():
        if not isinstance(key, str) or not isinstance(value, dict):
            raise AccountStoreError("accounts.json contains an invalid account entry")
        email = normalize_email(value.get("email") or key)
        if not email:
            raise AccountStoreError(f"Invalid account email: {key!r}")
        if email in accounts:
            raise AccountStoreError(f"Duplicate account email: {email}")
        account = dict(value)
        account["email"] = email
        account["name"] = clean_display_text(account.get("name"), "Google User") or "Google User"
        token = account.get("token_data")
        if token is not None and (not isinstance(token, str) or not decode_token(token)):
            raise AccountStoreError(f"Invalid saved token for {email}")
        claimed_email = extract_verified_google_email_claim(token)
        if claimed_email and claimed_email != email:
            raise AccountStoreError(f"Saved token email does not match {email}")
        quota_limits = account.get("quota_limits", {})
        if not isinstance(quota_limits, dict):
            raise AccountStoreError(f"Invalid quota limits for {email}")
        normalized_limits = {}
        for key, limit in quota_limits.items():
            if not isinstance(key, str) or not isinstance(limit, dict):
                raise AccountStoreError(f"Invalid quota limit for {email}")
            model = clean_display_text(limit.get("model"))
            family = limit.get("family")
            source = limit.get("source")
            if not model or family not in ("claude", "gemini", "gpt") or source not in ("log", "manual"):
                raise AccountStoreError(f"Invalid quota metadata for {email}")
            try:
                reset_at = _parse_utc_datetime(limit.get("reset_at")).isoformat()
                observed_at = _parse_utc_datetime(limit.get("observed_at")).isoformat()
            except (AttributeError, TypeError, ValueError):
                raise AccountStoreError(f"Invalid quota timestamp for {email}") from None
            normalized_limits[key] = {
                "model": model,
                "family": family,
                "reset_at": reset_at,
                "observed_at": observed_at,
                "source": source,
            }
            source_file = clean_display_text(limit.get("source_file"))
            if source_file:
                normalized_limits[key]["source_file"] = re.split(r"[\\/]", source_file)[-1]
        if normalized_limits:
            account["quota_limits"] = normalized_limits
        else:
            account.pop("quota_limits", None)
        if "legacy_quota" in account and not isinstance(account["legacy_quota"], dict):
            raise AccountStoreError(f"Invalid legacy quota for {email}")
        if "quota_snapshot" in account:
            account["quota_snapshot"] = _normalize_quota_snapshot(account["quota_snapshot"], email)
        accounts[email] = account
    return accounts


def migrate_legacy_quota(account):
    if account.get("quota_schema") == QUOTA_SCHEMA:
        return False
    legacy_keys = (
        "limit_reset", "limit_reset_claude", "limit_reset_gemini",
        "claude_pct", "gemini_pct",
    )
    legacy = dict(account.get("legacy_quota", {}))
    changed = False
    for key in legacy_keys:
        if key in account:
            legacy[key] = account.pop(key)
            changed = True
    if legacy:
        account["legacy_quota"] = legacy
    account["quota_schema"] = QUOTA_SCHEMA
    return True


def reconcile_log_limits(account, detected, evidence):
    limits = account.get("quota_limits", {})
    changed = False
    for key, limit in list(limits.items()):
        if limit.get("source") != "log":
            continue
        event = evidence.get(_model_identity(limit.get("model", "")))
        if not event:
            continue
        if _parse_utc_datetime(event["observed_at"]) >= _parse_utc_datetime(limit["observed_at"]):
            del limits[key]
            changed = True

    for key, limit in detected.items():
        existing = limits.get(key)
        if not existing or _parse_utc_datetime(limit["observed_at"]) > _parse_utc_datetime(existing["observed_at"]):
            limits[key] = limit
            changed = True

    if limits:
        account["quota_limits"] = limits
    else:
        account.pop("quota_limits", None)
    return changed


def load_accounts(sync_logs=True):
    try:
        revision = os.stat(ACCOUNTS_FILE).st_mtime_ns
        with open(ACCOUNTS_FILE, "r", encoding="utf-8") as f:
            accounts = _validate_accounts(json.load(f))
    except FileNotFoundError:
        revision = None
        accounts = Accounts()
    except (OSError, UnicodeDecodeError, json.JSONDecodeError, AccountStoreError) as exc:
        raise AccountStoreError(f"Cannot read {ACCOUNTS_FILE}: {exc}") from exc
    accounts.revision = revision

    if sync_logs and accounts:
        changed = False
        for acc in accounts.values():
            changed = migrate_legacy_quota(acc) or changed
        now = datetime.now(timezone.utc)

        for acc in accounts.values():
            limits = acc.get("quota_limits", {})
            for key, limit in list(limits.items()):
                dt = _parse_utc_datetime(limit["reset_at"])
                if dt <= now or (dt - now).total_seconds() > MAX_LIMIT_SECS:
                    del limits[key]
                    changed = True
            if not limits:
                acc.pop("quota_limits", None)

        detected_limits, evidence = auto_scan_logs_for_limits(include_evidence=True)
        for email, acc in accounts.items():
            changed = reconcile_log_limits(acc, detected_limits.get(email, {}), evidence.get(email, {})) or changed

        if changed:
            save_accounts(accounts)

    return accounts


def save_accounts(accounts):
    validated = _validate_accounts(accounts)
    payload = (json.dumps(validated, indent=2, ensure_ascii=False) + "\n").encode("utf-8")
    with _accounts_lock():
        try:
            current_revision = os.stat(ACCOUNTS_FILE).st_mtime_ns
        except FileNotFoundError:
            current_revision = None
        expected_revision = getattr(accounts, "revision", current_revision)
        if expected_revision != current_revision:
            raise AccountStoreError("accounts.json changed in another process; retry the command")
        if current_revision is not None:
            with open(ACCOUNTS_FILE, "rb") as f:
                _atomic_write_bytes(ACCOUNTS_FILE + ".bak", f.read())
        _atomic_write_bytes(ACCOUNTS_FILE, payload)
        accounts.revision = os.stat(ACCOUNTS_FILE).st_mtime_ns
