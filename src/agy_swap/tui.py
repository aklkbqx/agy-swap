"""Interactive terminal user interface (TUI) mode."""

import sys
import threading
import time

from agy_swap import (
    VERSION, TUI_AUTO_REFRESH_SECS, AccountStoreError,
    ORANGE, GREEN, RED, YELLOW, CYAN, GRAY, BRIGHT_WHITE, BOLD, RESET,
)
from agy_swap.commands import (
    cmd_add_flow, cmd_list, load_accounts, save_accounts,
    extract_email_from_token, refresh_quota_with_progress,
    refresh_quota_snapshots,
)
from agy_swap.credentials import apply_account_token, get_current_keychain_token
from agy_swap.display import (
    get_responsive_width, get_avatar_badge, get_account_limit_display,
    quota_groups, active_quota_limits, clean_display_text,
    truncate_visible, format_quota_bar, format_duration,
)
from agy_swap.quota import quota_snapshot_age, get_token_reset_info
from agy_swap.tty import get_key, get_key_with_timeout


def cmd_interactive(args):
    if not sys.stdin.isatty() or not sys.stdout.isatty():
        cmd_list(args)
        return

    accounts = load_accounts()
    quota_errors = refresh_quota_with_progress(accounts) if accounts else {}
    sys.stdout.write("\033[?1049h\033[?25l\033[H")
    sys.stdout.flush()

    resolved_cache = {}

    def get_active_details(token_str, accs):
        if not token_str:
            return None, None, False
        if token_str in resolved_cache:
            return resolved_cache[token_str]
        for email, acc in accs.items():
            if acc.get("token_data") == token_str:
                result = (email, acc.get("name", "Google User"), True)
                resolved_cache[token_str] = result
                return result

        email = extract_email_from_token(token_str)
        if email:
            for acc_email, acc in accs.items():
                if acc_email.lower() == email.lower():
                    result = (acc_email, acc.get("name", "Google User"), True)
                    resolved_cache[token_str] = result
                    return result
            result = (email, "Google User", False)
            resolved_cache[token_str] = result
            return result

        result = (None, None, False)
        resolved_cache[token_str] = result
        return result

    try:
        selected_idx = 0
        last_highlighted_idx = 0
        message = ""
        message_type = "info"
        last_refresh_time = time.time()
        bg_refresh_thread = None

        while True:
            try:
                accounts = load_accounts()
            except AccountStoreError as exc:
                message = f"Store error: {exc}"
                message_type = "error"
            current_token = get_current_keychain_token()
            active_email, active_name, is_saved = get_active_details(current_token, accounts)
            acc_list = list(accounts.values())

            items = []
            for acc in acc_list:
                items.append(("account", f"{acc['name']} <{acc['email']}>", acc))

            items.append(("action", "[a] Add Account", "add"))
            items.append(("action", "[d] Delete Account", "delete"))
            items.append(("action", "[q] Quit", "quit"))

            if selected_idx >= len(items):
                selected_idx = max(0, len(items) - 1)
            if selected_idx < 0:
                selected_idx = 0

            w = get_responsive_width()
            sep_line = f"{GRAY}{'─' * min(w, 80)}{RESET}"
            buf = []

            buf.append(f"{BOLD}{ORANGE}AGY SWAP{RESET} {GRAY}v{VERSION} · Google Antigravity Session Manager{RESET}")
            buf.append(sep_line)

            if active_email:
                tag = f"{GREEN}(Saved){RESET}" if is_saved else f"{YELLOW}(Unsaved, press 'a' to save){RESET}"
                avatar_active = get_avatar_badge(active_name, active_email)
                buf.append(f" {BOLD}Active:{RESET} {GREEN}●{RESET} {avatar_active} {BOLD}{active_name}{RESET} {GRAY}<{active_email}>{RESET} {tag}")
            else:
                buf.append(f" {BOLD}Active:{RESET} {GRAY}○ Not logged in{RESET}")

            buf.append("")
            
            highlighted_acc = acc_list[selected_idx] if selected_idx < len(acc_list) else None

            buf.append(f" {BOLD}ACCOUNTS{RESET} {GRAY}({len(acc_list)} total){RESET}")

            if not acc_list:
                buf.append(f"   {GRAY}No saved accounts. Press [a] to add an account.{RESET}")
            else:
                for idx_acc, acc in enumerate(acc_list):
                    is_selected = (idx_acc == selected_idx)
                    is_active = bool(active_email and acc["email"].lower() == active_email.lower())

                    cursor = f"{ORANGE}❯{RESET}" if is_selected else " "
                    dot = f"{GREEN}●{RESET}" if is_active else f"{GRAY}○{RESET}"
                    avatar = get_avatar_badge(acc.get("name", "Google User"), acc["email"])

                    limit_display = get_account_limit_display(acc, is_active=is_active, current_token_str=current_token)
                    idx_num = f"[{idx_acc + 1}]"

                    if is_selected:
                        buf.append(f" {cursor} {idx_num} {avatar} {dot} {BOLD}{BRIGHT_WHITE}{acc['name']}{RESET} {GRAY}<{acc['email']}>{RESET}  {limit_display}")
                    else:
                        buf.append(f" {cursor} {idx_num} {avatar} {dot} {acc['name']} {GRAY}<{acc['email']}>{RESET}  {limit_display}")

            if highlighted_acc:
                buf.append("")
                h_name = highlighted_acc.get("name", "Google User")
                buf.append(f" {BOLD}QUOTA TRACKER{RESET} {GRAY}(Highlighted: {h_name}){RESET}")

                groups = quota_groups(highlighted_acc)
                active_limits = active_quota_limits(highlighted_acc)
                if groups:
                    for group in groups:
                        buf.append(f"   {BOLD}{group['name']}{RESET}")
                        for bucket in group["buckets"]:
                            buf.append(f"     {bucket['name']:<15} {format_quota_bar(bucket)}")
                    age = quota_snapshot_age(highlighted_acc)
                    if age is not None:
                        buf.append(f"   {GRAY}Google quota synced {format_duration(age)} ago · [r] refresh{RESET}")
                elif active_limits:
                    buf.append(f"   {YELLOW}Google usage unavailable; using recent local cooldown errors{RESET}")
                    for remaining, limit in active_limits:
                        source = "manual" if limit["source"] == "manual" else "local log"
                        buf.append(f"   {limit['model']}  {RED}Limited{RESET} · resets in {format_duration(remaining)} {GRAY}({source}){RESET}")
                else:
                    reason = quota_errors.get(highlighted_acc.get("email"), "not synced")
                    buf.append(f"   {YELLOW}Usage unavailable{RESET} · {clean_display_text(reason)}")
                    buf.append(f"   {GRAY}No recent cooldown error observed · [r] retry{RESET}")

                tok_info = get_token_reset_info(highlighted_acc.get("token_data", ""))
                if tok_info:
                    rem_str = tok_info.get("remaining_str", "Unknown")
                    buf.append(f"   Session Token  {CYAN}{rem_str}{RESET}")

            buf.append("")
            buf.append(f" {BOLD}ACTIONS{RESET}")

            action_items = items[len(acc_list):]
            for act_idx, (itype, label, val) in enumerate(action_items, len(acc_list)):
                is_selected = (act_idx == selected_idx)
                cursor = f"{ORANGE}❯{RESET}" if is_selected else " "
                if is_selected:
                    buf.append(f" {cursor} {BOLD}{BRIGHT_WHITE}{label}{RESET}")
                else:
                    buf.append(f" {cursor} {GRAY}{label}{RESET}")

            buf.append("")
            buf.append(sep_line)
            if message:
                if message_type == "success":
                    buf.append(f" {GREEN}✓ {message}{RESET}")
                elif message_type == "error":
                    buf.append(f" {RED}✕ {message}{RESET}")
                else:
                    buf.append(f" {CYAN}› {message}{RESET}")
            else:
                buf.append(f" {GRAY}Navigate: [↑/↓] │ Select: [Enter] │ Shortcuts: [a] Add  [r] Refresh  [t] Tier  [d] Delete  [q] Quit{RESET}")

            sys.stdout.write("\033[H")
            for line in buf:
                sys.stdout.write(truncate_visible(line, w) + "\033[K\n")
            sys.stdout.write("\033[J")
            sys.stdout.flush()

            if bg_refresh_thread and not bg_refresh_thread.is_alive():
                bg_refresh_thread = None
            if not bg_refresh_thread and time.time() - last_refresh_time >= TUI_AUTO_REFRESH_SECS:
                def _auto_refresh():
                    try:
                        accs = load_accounts(sync_logs=False)
                        refresh_quota_snapshots(accs)
                    except Exception:
                        pass
                bg_refresh_thread = threading.Thread(target=_auto_refresh, daemon=True)
                bg_refresh_thread.start()
                last_refresh_time = time.time()

            elapsed = time.time() - last_refresh_time
            wait = max(0.5, TUI_AUTO_REFRESH_SECS - elapsed)
            key = get_key_with_timeout(wait)
            if key is None:
                continue
            message = ""

            if key == "up":
                selected_idx -= 1
                if selected_idx < 0:
                    selected_idx = len(items) - 1
            elif key == "down":
                selected_idx += 1
                if selected_idx >= len(items):
                    selected_idx = 0

            if selected_idx < len(acc_list):
                last_highlighted_idx = selected_idx

            if key == "up" or key == "down":
                pass
            elif key.isdigit():
                num = int(key)
                if 1 <= num <= len(acc_list):
                    selected_idx = num - 1
            elif key == "a":
                sys.stdout.write("\033[?1049l\033[?25h")
                sys.stdout.flush()
                try:
                    success, msg = cmd_add_flow()
                    input("\nPress Enter to return...")
                except KeyboardInterrupt:
                    success, msg = False, "Login cancelled"
                except AccountStoreError as exc:
                    success, msg = False, f"Store error: {exc}"
                resolved_cache.clear()
                message = msg
                message_type = "success" if success else "error"
                sys.stdout.write("\033[?1049h\033[?25l\033[H")
                sys.stdout.flush()
                continue
            elif key == "r":
                sys.stdout.write("\033[?1049l\033[?25h")
                sys.stdout.flush()
                try:
                    accs = load_accounts()
                    quota_errors = refresh_quota_with_progress(accs, force=True)
                    last_refresh_time = time.time()
                    message = "Usage refreshed" if not quota_errors else f"Usage refresh failed for {len(quota_errors)} account(s); cached data kept"
                    message_type = "success" if not quota_errors else "error"
                    resolved_cache.clear()
                except AccountStoreError as exc:
                    message = f"Refresh failed: {exc}"
                    message_type = "error"
                sys.stdout.write("\033[?1049h\033[?25l\033[H")
                sys.stdout.flush()
                continue
            elif key == "t" and selected_idx < len(acc_list):
                acc_target = items[selected_idx][2]
                email_t = acc_target["email"]
                try:
                    accs = load_accounts()
                    if email_t in accs:
                        if accs[email_t].get("quota_snapshot"):
                            message = f"Tier for {email_t} is synced from Google"
                            message_type = "info"
                            continue
                        current_plan = accs[email_t].get("plan") if accs[email_t].get("tier_source") == "manual" else None
                        plan = "Pro" if current_plan is None else "Free" if current_plan == "Pro" else None
                        if plan:
                            accs[email_t].update({"plan": plan, "tier": plan, "tier_source": "manual", "is_pro": plan == "Pro"})
                        else:
                            for field in ("plan", "tier", "tier_source", "is_pro"):
                                accs[email_t].pop(field, None)
                        save_accounts(accs)
                        resolved_cache.clear()
                        message = f"Set tier label for {email_t} to {plan or 'Unknown'}"
                        message_type = "success"
                except AccountStoreError as exc:
                    message = f"Store error: {exc}"
                    message_type = "error"
                continue
            elif key in ("q", "esc", "\x03", "\x04"):
                break
            elif key in ("d", "backspace", "delete") and selected_idx < len(acc_list):
                acc_to_del = items[selected_idx][2]
                email_del = acc_to_del["email"]
                buf[-1] = f" {RED}Confirm delete {email_del}? Press [y] to confirm, any key to cancel.{RESET}"
                sys.stdout.write("\033[H")
                for line in buf:
                    sys.stdout.write(truncate_visible(line, w) + "\033[K\n")
                sys.stdout.write("\033[J")
                sys.stdout.flush()
                if get_key().lower() == "y":
                    try:
                        accs = load_accounts()
                        if email_del in accs:
                            del accs[email_del]
                            save_accounts(accs)
                            resolved_cache.clear()
                            message = f"Removed account {email_del}"
                            message_type = "success"
                    except AccountStoreError as exc:
                        message = f"Store error: {exc}"
                        message_type = "error"
                else:
                    message = "Canceled"
                    message_type = "info"
                continue
            elif key in ("\r", "\n"):
                itype, label, val = items[selected_idx]

                if itype == "account":
                    sys.stdout.write("\033[?1049l\033[?25h")
                    sys.stdout.flush()
                    acc = val
                    print(f"Switching account to {BOLD}{acc['name']}{RESET} {GRAY}<{acc['email']}>{RESET}...")
                    with Spinner("Updating credentials..."):
                        success = apply_account_token(acc["token_data"], acc.get("email"))
                        time.sleep(0.4)
                    if success:
                        resolved_cache.clear()
                        message = f"Switched to {acc['email']}"
                        message_type = "success"
                    else:
                        message = f"Failed to switch to {acc['email']}"
                        message_type = "error"
                elif itype == "action":
                    sys.stdout.write("\033[?1049l\033[?25h")
                    sys.stdout.flush()

                    if val == "add":
                        try:
                            success, msg = cmd_add_flow()
                            input("\nPress Enter to return...")
                        except KeyboardInterrupt:
                            success, msg = False, "Login cancelled"
                        except AccountStoreError as exc:
                            success, msg = False, f"Store error: {exc}"
                        resolved_cache.clear()
                        message = msg
                        message_type = "success" if success else "error"

                    elif val == "delete":
                        if acc_list:
                            acc_to_del = acc_list[min(last_highlighted_idx, len(acc_list) - 1)]
                            email_del = acc_to_del["email"]
                            buf[-1] = f" {RED}Confirm delete {email_del}? Press [y] to confirm, any key to cancel.{RESET}"
                            sys.stdout.write("\033[H")
                            for line in buf:
                                sys.stdout.write(truncate_visible(line, w) + "\033[K\n")
                            sys.stdout.write("\033[J")
                            sys.stdout.flush()
                            if get_key().lower() == "y":
                                try:
                                    accs = load_accounts()
                                    if email_del in accs:
                                        del accs[email_del]
                                        save_accounts(accs)
                                        resolved_cache.clear()
                                        message = f"Removed account {email_del}"
                                        message_type = "success"
                                except AccountStoreError as exc:
                                    message = f"Store error: {exc}"
                                    message_type = "error"
                            else:
                                message = "Canceled"
                                message_type = "info"

                    elif val == "quit":
                        break

                sys.stdout.write("\033[?1049h\033[?25l\033[H")
                sys.stdout.flush()
                continue
    finally:
        sys.stdout.write("\033[?1049l\033[?25h")
        sys.stdout.flush()
