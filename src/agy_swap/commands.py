"""CLI subcommands and account management workflows."""

from datetime import datetime, timezone, timedelta
import json
import shutil
import subprocess
import sys
import time

from agy_swap import (
    QUOTA_SCHEMA, MAX_TOKEN_BYTES,
    GREEN, RED, YELLOW, GRAY, DARK_GRAY, CYAN, BOLD, RESET,
    AccountStoreError, AmbiguousAccountError,
    OAUTH_FILE, OAUTH_CREDS_FILE, GOOGLE_ACCOUNTS_FILE,
)
from agy_swap.credentials import (
    get_current_keychain_token, apply_account_token,
    _apply_account_token_unlocked, set_keychain_token,
    delete_keychain_token, clear_active_session,
    _clear_active_session_unlocked, _get_secure_token,
)
from agy_swap.display import (
    clean_display_text, normalize_email, format_duration, parse_duration_seconds,
    quota_groups, format_quota_bar, format_cooldown_bar, active_quota_limits,
    get_account_limit_display, get_avatar_badge, get_tier_badge,
    quota_wait_seconds, _parse_utc_datetime,
)
from agy_swap.oauth import decode_token, get_google_userinfo, extract_verified_google_email_claim
from agy_swap.quota import refresh_quota_snapshots, quota_snapshot_age
from agy_swap.storage import _snapshot_files, _restore_files, _session_lock
from agy_swap.store import load_accounts, save_accounts
from agy_swap.tty import Spinner


def _new_account(email, name, token_str):
    return {
        "email": email,
        "name": clean_display_text(name, "Google User") or "Google User",
        "token_data": token_str,
        "quota_schema": QUOTA_SCHEMA,
    }


def _save_token_as_account(token_str):
    token_json = decode_token(token_str)
    if not token_json or "token" not in token_json or "access_token" not in token_json["token"]:
        print(f"\n{RED}Invalid token structure.{RESET}")
        return False, "Invalid token structure"

    access_token = token_json["token"]["access_token"]
    new_userinfo = None
    with Spinner("Fetching Google profile & avatar..."):
        new_userinfo = get_google_userinfo(access_token)

    accounts = load_accounts(sync_logs=False)

    try:
        if new_userinfo and new_userinfo.get("email"):
            email = normalize_email(new_userinfo.get("email"))
            name = clean_display_text(new_userinfo.get("name"), "Google User") or "Google User"
            if not email:
                return False, "Google returned an invalid email"
            accounts[email] = _new_account(email, name, token_str)
            tier_label = get_tier_badge(accounts[email])
            save_accounts(accounts)
            print(f"\n{GREEN}✓ Successfully saved account: {name} <{email}>{RESET} [{tier_label}]")
            return True, f"Added {email}"
        else:
            email = normalize_email(input("Enter account email manually: "))
            if email:
                accounts[email] = _new_account(email, "Google User", token_str)
                tier_label = get_tier_badge(accounts[email])
                save_accounts(accounts)
                print(f"\n{GREEN}✓ Saved account: {email}{RESET} [{tier_label}]")
                return True, f"Added {email}"
            else:
                return False, "A valid email is required"
    except KeyboardInterrupt:
        print(f"\n{YELLOW}Cancelled. Token is still active in Keychain.{RESET}")
        return False, "Save cancelled"


def cmd_add_flow():
    with _session_lock():
        return _cmd_add_flow_unlocked()


def _cmd_add_flow_unlocked():
    current_token = get_current_keychain_token()
    backup_secure = _get_secure_token()
    backup_files = _snapshot_files((OAUTH_FILE, OAUTH_CREDS_FILE, GOOGLE_ACCOUNTS_FILE))

    def restore_previous_session():
        _restore_files(backup_files)
        if backup_secure:
            set_keychain_token(backup_secure)
        else:
            delete_keychain_token()

    if current_token:
        accounts = load_accounts(sync_logs=False)
        active_email = get_active_account_email(accounts, current_token)
        is_saved = bool(active_email and active_email.lower() in accounts)
        if not is_saved:
            token_json = decode_token(current_token)
            unsaved_email = None
            unsaved_name = "Google User"
            if token_json and "token" in token_json and "access_token" in token_json["token"]:
                userinfo = get_google_userinfo(token_json["token"]["access_token"])
                if userinfo:
                    unsaved_email = normalize_email(userinfo.get("email"))
                    unsaved_name = clean_display_text(userinfo.get("name"), "Google User") or "Google User"

            label = f"{unsaved_name} <{unsaved_email}>" if unsaved_email else "Unknown account"
            print(f"\n{YELLOW}⚠ Active session detected: {BOLD}{label}{RESET}")
            print(f"{GRAY}This account is not yet saved in agy-swap.{RESET}")
            choice = input(f"{CYAN}Save this account? [Y/n] (or 'n' to login a different account): {RESET}").strip().lower()
            if choice != "n":
                return _save_token_as_account(current_token)

    print(f"\n{BOLD}Add / Login Google Account{RESET}")
    print(f"{GRAY}1. A browser window will open to authenticate with Google.{RESET}")
    print(f"{GRAY}2. Complete login in Google Antigravity.{RESET}")
    input(f"{CYAN}Press Enter when ready to start login...{RESET}")

    if not _clear_active_session_unlocked():
        return False, "Could not clear the active session before login"

    agy_process = None
    agy_bin = shutil.which("agy")
    if agy_bin:
        try:
            agy_process = subprocess.Popen(
                [agy_bin],
                start_new_session=True,
            )
            print(f"\n{GREEN}✓ Launched 'agy' to open login prompt...{RESET}")
        except Exception:
            print(f"\n{YELLOW}Could not auto-launch 'agy'. Please run 'agy' in another terminal.{RESET}")
    else:
        print(f"\n{YELLOW}'agy' not found in PATH. Please run 'agy' manually in another terminal.{RESET}")

    print(f"{YELLOW}Waiting for login token in Keychain...{RESET}\n")

    timeout_secs = 120
    new_token = None
    try:
        with Spinner("Polling Keychain for new token..."):
            start_time = time.time()
            while time.time() - start_time < timeout_secs:
                t = get_current_keychain_token()
                if t and t != current_token:
                    new_token = t
                    break
                time.sleep(1.5)
    except KeyboardInterrupt:
        print(f"\n\n{YELLOW}Login cancelled by user.{RESET}")
        if current_token:
            restore_previous_session()
            print(f"{GRAY}Restored previous token.{RESET}")
        if agy_process:
            try:
                agy_process.terminate()
            except Exception:
                pass
        return False, "Login cancelled"
    finally:
        if agy_process:
            try:
                agy_process.terminate()
            except Exception:
                pass

    if not new_token:
        print(f"\n{RED}Timed out waiting for login ({timeout_secs}s).{RESET}")
        if current_token:
            restore_previous_session()
            print(f"{GRAY}Restored previous token.{RESET}")
        return False, "Login timed out"

    return _save_token_as_account(new_token)


def _read_token_stdin():
    stream = getattr(sys.stdin, "buffer", sys.stdin)
    raw = stream.read(MAX_TOKEN_BYTES + 1)
    if len(raw) > MAX_TOKEN_BYTES:
        raise ValueError(f"Token exceeds {MAX_TOKEN_BYTES} bytes")
    if isinstance(raw, bytes):
        raw = raw.decode("utf-8")
    return raw.strip()


def cmd_add(args):
    token_arg = getattr(args, "token", None)
    if token_arg is not None:
        if token_arg != "-":
            print(f"{RED}Inline tokens are unsafe. Pass the token on stdin with '--token -'.{RESET}", file=sys.stderr)
            sys.exit(2)
        try:
            token_str = _read_token_stdin()
        except (UnicodeDecodeError, ValueError) as exc:
            print(f"{RED}{exc}.{RESET}", file=sys.stderr)
            sys.exit(1)
        if not token_str:
            print(f"{RED}Empty token provided.{RESET}")
            sys.exit(1)
        token_json = decode_token(token_str)
        if not token_json or "token" not in token_json or "access_token" not in token_json["token"]:
            print(f"{RED}Could not parse token.{RESET}")
            sys.exit(1)
            
        access_token = token_json["token"]["access_token"]
        userinfo = None
        with Spinner("Fetching Google profile & avatar..."):
            userinfo = get_google_userinfo(access_token)
        
        if not userinfo:
            email = normalize_email(input("Enter email address manually: "))
            name = "Google User"
        else:
            email = normalize_email(userinfo.get("email"))
            name = clean_display_text(userinfo.get("name"), "Google User") or "Google User"

        if not email:
            print(f"{RED}Email address is required.{RESET}")
            sys.exit(1)

        accounts = load_accounts(sync_logs=False)
        accounts[email] = _new_account(email, name, token_str)
        save_accounts(accounts)
        print(f"{GREEN}✓ Saved account '{email}'.{RESET}")
    else:
        success, _ = cmd_add_flow()
        if not success:
            sys.exit(1)


def extract_email_from_token(token_str):
    claimed_email = extract_verified_google_email_claim(token_str)
    if claimed_email:
        return claimed_email
    token_json = decode_token(token_str)
    token_obj = token_json.get("token") if token_json else None
    if not isinstance(token_obj, dict):
        return None

    access_token = token_obj.get("access_token")
    if access_token:
        userinfo = get_google_userinfo(access_token)
        if userinfo and userinfo.get("email"):
            return normalize_email(userinfo.get("email"))
    return None


def get_active_account_email(accounts, current_token=None):
    if current_token is None:
        current_token = get_current_keychain_token()
    if not current_token:
        return None
    for email, acc in accounts.items():
        if acc.get("token_data") == current_token:
            return email
    token_email = extract_email_from_token(current_token)
    if token_email:
        for acc_email in accounts:
            if acc_email.lower() == token_email.lower():
                return acc_email
        return token_email
    return None


def resolve_account_target(target, accounts):
    if not target or not accounts:
        return None
    target_str = str(target).strip()
    keys = list(accounts.keys())
    if target_str.isdigit():
        idx = int(target_str) - 1
        if 0 <= idx < len(keys):
            return keys[idx]
        return None
    exact = [email for email in keys if target_str.lower() == email.lower()]
    if exact:
        return exact[0]
    matches = [email for email in keys if target_str.lower() in email.lower()]
    if len(matches) > 1:
        raise AmbiguousAccountError(f"Account target '{target}' matches: {', '.join(matches)}")
    if matches:
        return matches[0]
    return None


def account_cooldown_seconds(acc, now=None, family=None):
    quota_wait = quota_wait_seconds(acc, now, family)
    if quota_wait is not None:
        return quota_wait
    limits = [remaining for remaining, limit in active_quota_limits(acc, now) if family is None or limit.get("family") == family]
    return max(limits) if limits else None


def select_next_account(accounts, active_email=None, family=None):
    acc_list = list(accounts.values())
    if not acc_list:
        return None, False
    active_idx = next(
        (idx for idx, acc in enumerate(acc_list) if active_email and acc.get("email", "").lower() == active_email.lower()),
        -1,
    )
    ordered = [acc_list[(active_idx + offset) % len(acc_list)] for offset in range(1, len(acc_list) + 1)]
    waits = [(account, account_cooldown_seconds(account, family=family)) for account in ordered]
    for account, wait in waits:
        if wait == 0:
            return account, False
    for account, wait in waits:
        if wait is None:
            return account, None
    return min(waits, key=lambda item: item[1])[0], True


def refresh_quota_with_progress(accounts, force=False):
    def progress(index, total, account, state):
        if not sys.stdout.isatty():
            return
        width = 20
        filled = round(index / total * width) if total else width
        sys.stdout.write(
            f"\r{CYAN}Fetching usage [{GREEN}{'█' * filled}{DARK_GRAY}{'░' * (width - filled)}{CYAN}] "
            f"{index}/{total}{RESET} {account.get('name', account.get('email', 'account'))} ({state})"
        )
        sys.stdout.flush()

    errors = refresh_quota_snapshots(accounts, force=force, progress=progress)
    if sys.stdout.isatty():
        sys.stdout.write("\r\033[K")
        sys.stdout.flush()
    return errors


def cmd_list(args):
    accounts = load_accounts()
    if not accounts:
        print(f"{GRAY}No accounts added yet. Run 'agy-swap add' to save a Google account.{RESET}")
        return

    current_token = get_current_keychain_token()
    active_email = get_active_account_email(accounts, current_token)
    acc_list = list(accounts.values())

    print(f"\n{BOLD}MANAGED ACCOUNTS{RESET}")
    print(f"{GRAY}{'─'*65}{RESET}")
    if any(quota_groups(acc) for acc in acc_list):
        print(f"{GRAY}Quota comes from Google; grouped models share weekly and optional 5-hour limits.{RESET}")
    elif any(active_quota_limits(acc) for acc in acc_list):
        print(f"{GRAY}Google usage unavailable; cooldown bars below come from recent local errors.{RESET}")
    for i, acc in enumerate(acc_list, 1):
        email = acc["email"]
        name = acc.get("name", "Google User")
        avatar = get_avatar_badge(name, email)
        is_active = bool(active_email and email.lower() == active_email.lower())
        marker = f"{GREEN}●{RESET}" if is_active else f"{GRAY}○{RESET}"
        active_str = f" {GREEN}(Active){RESET}" if is_active else ""
        limit_info = get_account_limit_display(acc, is_active=is_active, current_token_str=current_token)
        print(f" {marker} [{i}] {avatar} {BOLD}{name}{RESET} {GRAY}<{email}>{RESET}{active_str}")
        print(f"       ↳ Status: {limit_info}")
        if quota_groups(acc):
            for group in quota_groups(acc):
                print(f"       ↳ {group['name']}")
                for bucket in group["buckets"]:
                    print(f"          {bucket['name']}: {format_quota_bar(bucket)}")
        else:
            for remaining, limit in active_quota_limits(acc):
                progress_bar = format_cooldown_bar(limit)
                if progress_bar:
                    print(f"       ↳ {limit['model']}: {progress_bar} · resets in {format_duration(remaining)}")
        if getattr(args, "verbose", False):
            age = quota_snapshot_age(acc)
            if age is not None:
                print(f"       ↳ Google quota synced {format_duration(age)} ago")
            for _, limit in active_quota_limits(acc):
                observed = _parse_utc_datetime(limit["observed_at"]).strftime("%Y-%m-%d %H:%M:%S UTC")
                source = "manual" if limit["source"] == "manual" else f"local log {limit.get('source_file', '(unknown file)')}"
                print(f"       ↳ {limit['model']}: observed {observed} · source {source}")
    print(f"{GRAY}{'─'*65}{RESET}\n")


def cmd_next(args):
    family = getattr(args, "family", None)
    label = f"{family.title()} " if family else ""
    accounts = load_accounts()
    if not accounts:
        print(f"{GRAY}No accounts found. Add one with 'agy-swap add' first.{RESET}")
        sys.exit(1)
    refresh_quota_snapshots(accounts)
    with _session_lock():
        current_token = get_current_keychain_token()
        active_email = get_active_account_email(accounts, current_token)
        next_acc, all_limited = select_next_account(accounts, active_email, family)
        if all_limited:
            wait = format_duration(account_cooldown_seconds(next_acc, family=family))
            print(f"{YELLOW}All accounts have an observed {label}limit; selecting the shortest wait ({wait}).{RESET}")
            reason = f"shortest observed {label}limit"
        elif all_limited is None:
            print(f"{YELLOW}No account has confirmed available {label}quota; selecting the next account with unverified usage.{RESET}")
            reason = f"unverified {label}quota"
        else:
            reason = f"available {label}quota"

        print(f"Auto-rotating to account with {reason}: {BOLD}{next_acc['name']}{RESET} {GRAY}<{next_acc['email']}>{RESET}...")
        with Spinner("Updating credentials..."):
            success = _apply_account_token_unlocked(next_acc["token_data"], next_acc.get("email"))
        
    if success:
        print(f"{GREEN}✓ Successfully auto-rotated to {next_acc['email']}.{RESET}")
    else:
        print(f"{RED}✕ Failed to rotate account.{RESET}")
        sys.exit(1)


def cmd_switch(args):
    from agy_swap.tui import cmd_interactive
    accounts = load_accounts()
    if not accounts:
        print(f"{GRAY}No accounts found. Add one with 'agy-swap add' first.{RESET}")
        sys.exit(1)

    target = getattr(args, "account", None)
    if not target:
        if sys.stdin.isatty():
            cmd_interactive(args)
            return
        else:
            print(f"{RED}Interactive mode requires TTY.{RESET}")
            sys.exit(1)

    selected_email = resolve_account_target(target, accounts)

    if not selected_email:
        print(f"{RED}Account '{target}' not found.{RESET}")
        sys.exit(1)

    acc = accounts[selected_email]
    print(f"Switching to {BOLD}{acc['name']}{RESET} {GRAY}<{acc['email']}>{RESET}...")
    with Spinner("Updating credentials..."):
        success = apply_account_token(acc["token_data"], acc.get("email"))
    if success:
        print(f"{GREEN}✓ Successfully switched to {acc['email']}.{RESET}")
    else:
        print(f"{RED}✕ Failed to switch account.{RESET}")
        sys.exit(1)


def cmd_set_limit(args):
    accounts = load_accounts()
    if not accounts:
        print(f"{GRAY}No accounts found.{RESET}")
        sys.exit(1)

    target = getattr(args, "account", None)
    duration_str = getattr(args, "duration", None)
    model_group = getattr(args, "group", "claude") or "claude"
    
    if not target:
        target = input("Enter account email or index: ").strip()
    
    selected_email = resolve_account_target(target, accounts)

    if not selected_email:
        print(f"{RED}Account '{target}' not found.{RESET}")
        sys.exit(1)

    if not duration_str:
        duration_str = input("Enter cooldown duration (e.g. '4h10m', '6d', or 'reset'): ").strip()

    seconds = parse_duration_seconds(duration_str)
    if seconds is None:
        print(f"{RED}Invalid duration format or duration exceeds 7 days.{RESET}")
        sys.exit(1)

    limits = accounts[selected_email].setdefault("quota_limits", {})
    key = f"manual:{model_group}"
    if seconds == 0:
        limits.pop(key, None)
        if not limits:
            accounts[selected_email].pop("quota_limits", None)
        save_accounts(accounts)
        print(f"{GREEN}✓ Cleared rate limit cooldown for '{selected_email}'.{RESET}")
        return

    now = datetime.now(timezone.utc)
    reset_time = now + timedelta(seconds=seconds)
    limits[key] = {
        "model": model_group.upper() if model_group == "gpt" else model_group.title(),
        "family": model_group,
        "reset_at": reset_time.isoformat(),
        "observed_at": now.isoformat(),
        "source": "manual",
    }
    save_accounts(accounts)
    
    print(f"{GREEN}✓ Set {model_group.title()} rate limit cooldown for '{selected_email}' ({reset_time.strftime('%H:%M:%S UTC')}).{RESET}")


def cmd_limits(args):
    accounts = load_accounts()
    errors = {}
    if accounts:
        errors = refresh_quota_with_progress(accounts, force=getattr(args, "refresh", False))
    cmd_list(args)
    if errors:
        print(f"{YELLOW}Usage refresh failed for {len(errors)} account(s); cached data kept.{RESET}", file=sys.stderr)


def cmd_remove(args):
    from agy_swap.tui import cmd_interactive
    accounts = load_accounts()
    if not accounts:
        print(f"{GRAY}No accounts found.{RESET}")
        sys.exit(1)

    target = getattr(args, "account", None)
    if not target:
        if sys.stdin.isatty():
            cmd_interactive(args)
            return
        else:
            print(f"{RED}Interactive mode requires TTY.{RESET}")
            sys.exit(1)

    selected_email = resolve_account_target(target, accounts)

    if not selected_email:
        print(f"{RED}Account '{target}' not found.{RESET}")
        sys.exit(1)

    confirm = input(f"Are you sure you want to remove '{selected_email}'? [y/N]: ").strip().lower()
    if confirm == "y":
        del accounts[selected_email]
        save_accounts(accounts)
        print(f"{GREEN}✓ Removed account '{selected_email}'.{RESET}")


def cmd_status(args):
    current_token = get_current_keychain_token()
    if not current_token:
        print(f"{GRAY}Status: Not logged in{RESET}")
        return

    accounts = load_accounts()
    active_email = get_active_account_email(accounts, current_token)

    if active_email and active_email in accounts:
        acc = accounts[active_email]
        limit_info = get_account_limit_display(acc, is_active=True, current_token_str=current_token)
        avatar = get_avatar_badge(acc.get("name", ""), active_email)
        print(f"Active Account: {avatar} {GREEN}● {acc['name']}{RESET} {GRAY}<{acc['email']}>{RESET}")
        print(f"Status: {limit_info}")
    else:
        email = active_email or "Unknown Session"
        print(f"Active Account: {YELLOW}● {email}{RESET} {GRAY}(Unsaved in agy-swap){RESET}")


def cmd_logout(args):
    current_token = get_current_keychain_token()
    if not clear_active_session():
        print(f"{RED}✕ Could not clear every active credential store.{RESET}", file=sys.stderr)
        sys.exit(1)
    if current_token:
        print(f"{GREEN}✓ Logged out and removed active OAuth credentials.{RESET}")
    else:
        print(f"{GRAY}Already logged out; removed any stale credential files.{RESET}")
