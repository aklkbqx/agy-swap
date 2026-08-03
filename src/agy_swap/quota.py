"""Google Cloud Code API quota fetching and batch background refresh."""

from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone

from agy_swap import TIER_NAMES, QUOTA_CACHE_SECS, QuotaFetchError, AccountStoreError
from agy_swap.display import clean_display_text, _parse_utc_datetime
from agy_swap.oauth import _refresh_access_token, _cloud_code_post, extract_verified_google_email_claim
from agy_swap.store import _normalize_quota_snapshot, save_accounts


def fetch_account_quota(account):
    access_token = _refresh_access_token(account.get("token_data"))
    claimed_email = extract_verified_google_email_claim(account.get("token_data"))
    if claimed_email and claimed_email != account.get("email"):
        raise QuotaFetchError("refreshed token identity does not match the account")

    account_info = _cloud_code_post(access_token, "loadCodeAssist", {"metadata": {"ideType": "ANTIGRAVITY"}})
    project = account_info.get("cloudaicompanionProject")
    if not isinstance(project, str) or not project:
        raise QuotaFetchError("Google returned no Code Assist project")
    summary = _cloud_code_post(access_token, "retrieveUserQuotaSummary", {"project": project})

    tier = account_info.get("paidTier") or account_info.get("currentTier")
    if not isinstance(tier, dict) or not tier.get("id"):
        raise QuotaFetchError("Google returned no account tier")
    groups = []
    for raw_group in summary.get("groups", []):
        if not isinstance(raw_group, dict):
            continue
        buckets = []
        group_id = None
        for raw_bucket in raw_group.get("buckets", []):
            if not isinstance(raw_bucket, dict):
                continue
            bucket_id = clean_display_text(raw_bucket.get("bucketId"))
            window = raw_bucket.get("window")
            fraction = raw_bucket.get("remainingFraction")
            reset_at = raw_bucket.get("resetTime")
            if bucket_id.startswith("gemini-"):
                group_id = "gemini"
            elif bucket_id.startswith("3p-"):
                group_id = "third_party"
            if window not in ("weekly", "5h") or isinstance(fraction, bool) or not isinstance(fraction, (int, float)) or not isinstance(reset_at, str):
                continue
            buckets.append({
                "id": bucket_id,
                "name": clean_display_text(raw_bucket.get("displayName"), window),
                "window": window,
                "remaining_fraction": min(1.0, max(0.0, float(fraction))),
                "reset_at": reset_at,
            })
        if group_id and buckets:
            groups.append({
                "id": group_id,
                "name": clean_display_text(raw_group.get("displayName"), "Model group"),
                "buckets": buckets,
            })
    if not groups:
        raise QuotaFetchError("Google returned no quota groups")

    tier_id = clean_display_text(tier.get("id"))
    snapshot = {
        "observed_at": datetime.now(timezone.utc).isoformat(),
        "tier": {"id": tier_id, "name": TIER_NAMES.get(tier_id, clean_display_text(tier.get("name"), tier_id))},
        "groups": groups,
    }
    try:
        return _normalize_quota_snapshot(snapshot, account.get("email", "account"))
    except AccountStoreError:
        raise QuotaFetchError("Google returned invalid quota data") from None


def quota_snapshot_age(account, now=None):
    snapshot = account.get("quota_snapshot")
    if not isinstance(snapshot, dict):
        return None
    try:
        return max(0, ((now or datetime.now(timezone.utc)) - _parse_utc_datetime(snapshot["observed_at"])).total_seconds())
    except (KeyError, AttributeError, TypeError, ValueError):
        return None


def refresh_quota_snapshots(accounts, force=False, progress=None):
    errors = {}
    changed = False
    now = datetime.now(timezone.utc)
    total = len(accounts)

    to_fetch = []
    cached_count = 0
    for email, account in accounts.items():
        age = quota_snapshot_age(account, now)
        if not force and age is not None and age < QUOTA_CACHE_SECS:
            cached_count += 1
            if progress:
                progress(cached_count, total, account, "cached")
        else:
            to_fetch.append((email, account))

    if to_fetch:
        def _fetch_one(item):
            email, account = item
            try:
                snapshot = fetch_account_quota(account)
                return email, snapshot, None
            except QuotaFetchError as exc:
                return email, None, str(exc)

        workers = min(len(to_fetch), 8)
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = {executor.submit(_fetch_one, item): item[0] for item in to_fetch}
            done_count = cached_count
            for future in as_completed(futures):
                email, snapshot, error = future.result()
                done_count += 1
                if snapshot:
                    accounts[email]["quota_snapshot"] = snapshot
                    changed = True
                    status = "synced"
                else:
                    errors[email] = error
                    status = "failed"
                if progress:
                    progress(done_count, total, accounts[email], status)

    if changed:
        save_accounts(accounts)
    return errors


def get_token_reset_info(token_data_str):
    from agy_swap.oauth import decode_token
    token_json = decode_token(token_data_str)
    if not token_json or "token" not in token_json:
        return None
    token_obj = token_json["token"]
    expiry_str = token_obj.get("expiry")
    has_refresh = bool(token_obj.get("refresh_token"))

    try:
        if expiry_str:
            dt = _parse_utc_datetime(expiry_str)
        elif token_obj.get("expiry_date"):
            dt = datetime.fromtimestamp(float(token_obj["expiry_date"]) / 1000, tz=timezone.utc)
        else:
            return None
        now = datetime.now(timezone.utc)
        diff = (dt - now).total_seconds()
        if diff > 0:
            mins = int(diff // 60)
            secs = int(diff % 60)
            if mins >= 60:
                hours = mins // 60
                rem_mins = mins % 60
                return {"status": "active", "remaining_str": f"{hours}h {rem_mins}m", "seconds": diff, "expiry_dt": dt}
            return {"status": "active", "remaining_str": f"{mins}m {secs}s", "seconds": diff, "expiry_dt": dt}
        else:
            if has_refresh:
                return {"status": "refresh_available", "remaining_str": "Access expired · refresh token available", "seconds": diff, "expiry_dt": dt}
            else:
                abs_mins = int(abs(diff) // 60)
                return {"status": "expired", "remaining_str": f"Expired {abs_mins}m ago", "seconds": diff, "expiry_dt": dt}
    except (TypeError, ValueError, OverflowError):
        return None
