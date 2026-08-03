"""Output formatting, terminal display helpers, and duration utilities."""

from datetime import datetime, timezone, timedelta
import os
import re
import shutil
import sys
import unicodedata

from agy_swap import (
    ORANGE, GREEN, BLUE, RED, YELLOW, CYAN, GRAY, DARK_GRAY, BRIGHT_WHITE,
    BOLD, RESET, ANSI_ESCAPE, MAX_LIMIT_SECS, TIER_NAMES,
)


def configure_output():
    if sys.stdout.isatty() and "NO_COLOR" not in os.environ:
        return
    for name in ("ORANGE", "GREEN", "BLUE", "RED", "YELLOW", "CYAN", "GRAY", "DARK_GRAY", "BRIGHT_WHITE", "BOLD", "RESET"):
        globals()[name] = ""


def clean_display_text(value, fallback=""):
    text = str(value or fallback)
    return "".join(ch for ch in text if ch in "\t " or unicodedata.category(ch)[0] != "C").strip()


def normalize_email(value):
    email = clean_display_text(value).lower()
    if not re.fullmatch(r"[^@\s]+@[^@\s]+\.[^@\s]+", email):
        return None
    return email


def _parse_utc_datetime(value):
    dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def format_duration(seconds):
    mins = max(0, int(seconds // 60))
    days, mins = divmod(mins, 24 * 60)
    hours, mins = divmod(mins, 60)
    if days:
        return f"{days}d {hours}h {mins}m"
    return f"{hours}h {mins}m" if hours else f"{mins}m"


def parse_duration_seconds(duration_str):
    if not duration_str:
        return None
    duration_str = duration_str.strip().lower()
    if duration_str in ("reset", "clear", "0", "none"):
        return 0
    if duration_str.isdigit():
        total = int(duration_str) * 60
    else:
        match = re.fullmatch(r"\s*(?:(\d+)\s*d)?\s*(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?\s*(?:(\d+)\s*s)?\s*", duration_str)
        if not match or not any(match.groups()):
            return None
        days, hours, mins, secs = (int(value or 0) for value in match.groups())
        total = days * 86400 + hours * 3600 + mins * 60 + secs
    return total if 0 <= total <= MAX_LIMIT_SECS else None


def quota_groups(acc):
    snapshot = acc.get("quota_snapshot")
    groups = snapshot.get("groups") if isinstance(snapshot, dict) else None
    return groups if isinstance(groups, list) else []


def format_quota_bar(bucket, now=None, width=12):
    fraction = min(1.0, max(0.0, float(bucket["remaining_fraction"])))
    filled = round(fraction * width)
    color = RED if fraction <= 0.1 else YELLOW if fraction <= 0.3 else GREEN
    bar = f"{color}{'█' * filled}{DARK_GRAY}{'░' * (width - filled)}{RESET}"
    try:
        remaining = (_parse_utc_datetime(bucket["reset_at"]) - (now or datetime.now(timezone.utc))).total_seconds()
        reset = f" · resets in {format_duration(remaining)}" if remaining > 0 else " · refresh due"
    except (KeyError, AttributeError, TypeError, ValueError):
        reset = ""
    return f"[{bar}] {fraction * 100:.2f}% remaining{reset}"


def quota_wait_seconds(acc, now=None, family=None):
    groups = quota_groups(acc)
    if not groups:
        return None
    now = now or datetime.now(timezone.utc)
    group_id = "gemini" if family == "gemini" else "third_party" if family in ("claude", "gpt") else None
    matched = False
    waits = []
    for group in groups:
        if group_id and group.get("id") != group_id:
            continue
        matched = True
        for bucket in group.get("buckets", []):
            if bucket.get("remaining_fraction", 1) > 0:
                continue
            try:
                waits.append(max(0, (_parse_utc_datetime(bucket["reset_at"]) - now).total_seconds()))
            except (KeyError, AttributeError, TypeError, ValueError):
                pass
    return max(waits, default=0) if matched else None


def format_cooldown_bar(limit, now=None, width=12):
    if not limit:
        return f"[{DARK_GRAY}{'░' * width}{RESET}] No active cooldown observed"
    now = now or datetime.now(timezone.utc)
    try:
        observed_at = _parse_utc_datetime(limit["observed_at"])
        reset_at = _parse_utc_datetime(limit["reset_at"])
    except (KeyError, TypeError, ValueError):
        return None
    total = (reset_at - observed_at).total_seconds()
    if total <= 0:
        return None
    ratio = min(1.0, max(0.0, (reset_at - now).total_seconds() / total))
    filled = round(ratio * width)
    bar = f"{RED}{'█' * filled}{DARK_GRAY}{'░' * (width - filled)}{RESET}"
    return f"[{bar}] {ratio * 100:.1f}% time left"


def active_quota_limits(acc, now=None):
    now = now or datetime.now(timezone.utc)
    active = []
    for limit in acc.get("quota_limits", {}).values():
        try:
            reset_at = _parse_utc_datetime(limit["reset_at"])
        except (KeyError, TypeError, ValueError):
            continue
        remaining = (reset_at - now).total_seconds()
        if remaining > 0:
            active.append((remaining, limit))
    return sorted(active, key=lambda item: item[0])


def get_tier_badge(acc):
    snapshot = acc.get("quota_snapshot")
    tier = snapshot.get("tier") if isinstance(snapshot, dict) else None
    if isinstance(tier, dict) and tier.get("name"):
        plan = clean_display_text(tier["name"])
        color = GRAY if tier.get("id") == "free-tier" else ORANGE
        return f"{color}{plan}{RESET}"
    source = acc.get("tier_source")
    plan = clean_display_text(acc.get("plan") or acc.get("tier")).title()
    if plan not in ("Pro", "Starter", "Free") or source != "manual":
        return f"{GRAY}Unknown{RESET}"
    color = ORANGE if plan == "Pro" else GRAY
    return f"{color}{plan} (manual){RESET}"


def get_account_limit_display(acc, is_active=False, current_token_str=None):
    tier = get_tier_badge(acc)
    buckets = [bucket for group in quota_groups(acc) for bucket in group.get("buckets", [])]
    if buckets:
        lowest = min(buckets, key=lambda bucket: bucket["remaining_fraction"])
        fraction = lowest["remaining_fraction"]
        if fraction <= 0:
            try:
                remaining = (_parse_utc_datetime(lowest["reset_at"]) - datetime.now(timezone.utc)).total_seconds()
            except (KeyError, AttributeError, TypeError, ValueError):
                remaining = 0
            return f"[{tier}] {RED}Limited{RESET} · {lowest['name']} resets in {format_duration(remaining)}"
        color = RED if fraction <= 0.1 else YELLOW if fraction <= 0.3 else GREEN
        return f"[{tier}] {color}{fraction * 100:.0f}% lowest remaining{RESET}"
    active = active_quota_limits(acc)
    if active:
        parts = [f"{RED}! {limit['model']} ({format_duration(remaining)}){RESET}" for remaining, limit in active]
        return f"[{tier}] " + " ".join(parts)
    return f"[{tier}] {GRAY}Usage unavailable · no recent cooldown error{RESET}"


def get_avatar_badge(name, email):
    name_str = clean_display_text(name)
    parts = name_str.split()
    if len(parts) >= 2 and parts[0] and parts[1]:
        initials = (parts[0][0] + parts[1][0]).upper()
    elif len(parts) == 1 and parts[0]:
        initials = parts[0][:2].upper()
    else:
        initials = email[:2].upper() if email else "GU"

    if not sys.stdout.isatty() or "NO_COLOR" in os.environ:
        return initials

    bg_colors = [166, 172, 64, 71, 133, 167, 31, 68]
    bg_code = bg_colors[sum(email.encode("utf-8")) % len(bg_colors)]
    return f"\033[48;5;{bg_code}m\033[38;5;255m\033[1m {initials} \033[0m"


def visible_len(text):
    return sum(
        0 if unicodedata.combining(ch) else 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1
        for ch in ANSI_ESCAPE.sub("", text)
    )


def truncate_visible(text, width):
    if visible_len(text) <= width:
        return text
    result = []
    used = 0
    index = 0
    while index < len(text) and used < max(0, width - 1):
        match = ANSI_ESCAPE.match(text, index)
        if match:
            result.append(match.group(0))
            index = match.end()
            continue
        ch = text[index]
        char_width = 0 if unicodedata.combining(ch) else 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1
        if used + char_width > width - 1:
            break
        result.append(ch)
        used += char_width
        index += 1
    return "".join(result) + "…" + RESET


def get_responsive_width():
    try:
        cols, _ = shutil.get_terminal_size((80, 24))
        return max(28, min(cols - 2, 110))
    except Exception:
        return 76
