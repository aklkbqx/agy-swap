"""Local log scanning and rate-limit cooldown detection."""

from datetime import datetime, timezone, timedelta
import glob
import json
import os
import platform
import re
import time

import agy_swap
from agy_swap import MAX_LIMIT_SECS, LOG_SCAN_BYTES, LOG_TOTAL_SCAN_BYTES
from agy_swap.display import clean_display_text, normalize_email, parse_duration_seconds, _parse_utc_datetime


# Package modules have their own globals; keep the log-scan cache here rather
# than relying on the bundled single-file namespace.
_LOG_SCAN_CACHE = None


def find_antigravity_logs():
    log_dirs = [
        os.path.expanduser("~/Library/Logs/Google Antigravity"),
        os.path.expanduser("~/Library/Logs/Antigravity"),
        os.path.expanduser("~/.config/antigravity/logs"),
        os.path.expanduser("~/.gemini/antigravity-cli/logs"),
        os.path.expanduser("~/.gemini/antigravity-cli/log"),
        os.path.expanduser("~/Library/Application Support/Antigravity IDE/logs"),
    ]
    if platform.system() == "Windows":
        appdata = os.getenv("LOCALAPPDATA")
        if appdata:
            log_dirs.append(os.path.join(appdata, "Google", "Antigravity", "logs"))
            log_dirs.append(os.path.join(appdata, "Antigravity", "logs"))

    cli_log = os.path.expanduser("~/.gemini/antigravity-cli/cli.log")

    found_files = []
    if os.path.isfile(cli_log):
        found_files.append(cli_log)
    for d in log_dirs:
        if os.path.exists(d):
            found_files.extend(glob.glob(os.path.join(d, "*.log")))
            found_files.extend(glob.glob(os.path.join(d, "**", "*.log"), recursive=True))

    candidates = []
    for path in dict.fromkeys(found_files):
        try:
            candidates.append((os.path.getmtime(path), path))
        except OSError:
            pass
    return [path for _, path in sorted(candidates, reverse=True)]


def _log_signature(log_files):
    signature = []
    for path in log_files:
        try:
            stat = os.stat(path)
            signature.append((path, stat.st_mtime_ns, stat.st_size))
        except OSError:
            pass
    return tuple(signature)


def _recent_log_files(log_files):
    selected = []
    scanned_bytes = 0
    cutoff = time.time() - MAX_LIMIT_SECS
    for path in log_files:
        try:
            stat = os.stat(path)
        except OSError:
            continue
        if stat.st_mtime < cutoff:
            continue
        scan_bytes = min(stat.st_size, LOG_SCAN_BYTES)
        if selected and scanned_bytes + scan_bytes > LOG_TOTAL_SCAN_BYTES:
            break
        selected.append(path)
        scanned_bytes += scan_bytes
    return selected


def _iter_log_lines(path):
    with open(path, "rb") as f:
        size = os.fstat(f.fileno()).st_size
        offset = max(0, size - LOG_SCAN_BYTES)
        f.seek(offset)
        if offset:
            f.readline()
        for raw in f:
            yield raw.decode("utf-8", errors="ignore")


def _quota_key(label):
    return re.sub(r"[^a-z0-9]+", "-", label.lower()).strip("-")


def _model_family(label):
    label_lower = label.lower()
    if re.search(r"\b(gemini|flash)\b", label_lower):
        return "gemini"
    if re.search(r"\b(claude|anthropic)\b", label_lower):
        return "claude"
    if re.search(r"\b(gpt|openai)\b", label_lower):
        return "gpt"
    return None


def _model_identity(label):
    return re.sub(r"-thinking$", "", _quota_key(label))


def _detect_resolved_model_context(line):
    match = re.search(r"\bresolving model\s+(.+?)\s*$", line, re.IGNORECASE)
    if not match:
        return None
    label = clean_display_text(match.group(1))
    family = _model_family(label)
    return (_quota_key(label), label, family) if family else None


def _detect_model_context(line):
    label_match = re.search(r"\blabel\s*=\s*[\"']([^\"']+)", line, re.IGNORECASE)
    if not label_match:
        return None
    label = clean_display_text(label_match.group(1))
    family = _model_family(label)
    return (_quota_key(label), label, family) if family else None


def _parse_log_timestamp(line, log_file):
    ide = re.search(r"(\d{4})-(\d{2})-(\d{2})\s+(\d{2}):(\d{2}):(\d{2})", line)
    try:
        if ide:
            parts = [int(value) for value in ide.groups()]
            return datetime(*parts).astimezone().astimezone(timezone.utc)

        glog = re.search(r"\b[IWEF](\d{2})(\d{2})\s+(\d{2}):(\d{2}):(\d{2})", line)
        if not glog:
            return None
        month, day, hour, minute, second = [int(value) for value in glog.groups()]
        reference = datetime.fromtimestamp(os.path.getmtime(log_file)).astimezone()
        candidates = [
            datetime(year, month, day, hour, minute, second).astimezone()
            for year in (reference.year - 1, reference.year, reference.year + 1)
        ]
        return min(candidates, key=lambda value: abs((value - reference).total_seconds())).astimezone(timezone.utc)
    except (OSError, ValueError):
        return None


def auto_scan_logs_for_limits(include_evidence=False):
    global _LOG_SCAN_CACHE
    log_files = _recent_log_files(find_antigravity_logs())
    if not log_files:
        return ({}, {}) if include_evidence else {}

    signature = _log_signature(log_files)
    if _LOG_SCAN_CACHE and _LOG_SCAN_CACHE[0] == signature:
        limits = json.loads(json.dumps(_LOG_SCAN_CACHE[1]))
        evidence = json.loads(json.dumps(_LOG_SCAN_CACHE[2]))
        return (limits, evidence) if include_evidence else limits

    email_regex = re.compile(r"applyAuthResult:\s*email=([^\s,]+)", re.IGNORECASE)
    error_regex = re.compile(r"RESOURCE_EXHAUSTED.*?Resets\s+in\s+((?:\d+\s*[dhms]\s*)+)", re.IGNORECASE)
    send_regex = re.compile(r"Sending user message to conversation\s+([0-9a-f-]+)", re.IGNORECASE)
    complete_regex = re.compile(r"Stream completed for\s+([0-9a-f-]+)", re.IGNORECASE)
    evidence = {}
    now = datetime.now(timezone.utc)

    def record_evidence(email, model, state, observed_at, limit=None):
        identity = _model_identity(model[1])
        events = evidence.setdefault(email, {})
        existing = events.get(identity)
        if existing and _parse_utc_datetime(existing["observed_at"]) >= observed_at:
            return
        events[identity] = {
            "state": state,
            "observed_at": observed_at.isoformat(),
            "model": model[1],
            "family": model[2],
            "key": model[0],
        }
        if limit:
            events[identity]["limit"] = limit

    for log_file in log_files:
        current_email = None
        current_model = None
        pending_resolved = None
        model_labels = {}
        pending_requests = {}
        try:
            for line in _iter_log_lines(log_file):
                em_match = email_regex.search(line)
                if em_match:
                    current_email = normalize_email(em_match.group(1))
                    current_model = None
                    pending_resolved = None

                resolved = _detect_resolved_model_context(line)
                if resolved:
                    pending_resolved = resolved[0]
                    current_model = model_labels.get(resolved[0], resolved)

                labeled = _detect_model_context(line)
                if labeled:
                    key = pending_resolved if pending_resolved and current_model and current_model[2] == labeled[2] else labeled[0]
                    current_model = (key, labeled[1], labeled[2])
                    model_labels[key] = current_model
                    pending_resolved = None

                send_match = send_regex.search(line)
                if send_match and current_email and current_model:
                    sent_at = _parse_log_timestamp(line, log_file)
                    if sent_at:
                        pending_requests[send_match.group(1)] = {
                            "email": current_email,
                            "model": current_model,
                            "sent_at": sent_at,
                            "response": False,
                            "failed": False,
                        }

                latest_request = max(pending_requests.values(), key=lambda request: request["sent_at"], default=None)
                if latest_request and "streamGenerateContent" in line and "ResponseID:" in line:
                    latest_request["response"] = True

                err_match = error_regex.search(line)
                if err_match:
                    if latest_request:
                        latest_request["failed"] = True
                    email = latest_request["email"] if latest_request else current_email
                    model = latest_request["model"] if latest_request else current_model
                    total_secs = parse_duration_seconds(err_match.group(1))
                    log_dt = _parse_log_timestamp(line, log_file)
                    if email and model and total_secs and log_dt:
                        reset_dt = log_dt + timedelta(seconds=total_secs)
                        if reset_dt > now and (reset_dt - now).total_seconds() <= MAX_LIMIT_SECS:
                            model_key, model_label, family = model
                            limit = {
                                "model": model_label,
                                "family": family,
                                "reset_at": reset_dt.isoformat(),
                                "observed_at": log_dt.isoformat(),
                                "source": "log",
                                "source_file": clean_display_text(os.path.basename(log_file)),
                            }
                            record_evidence(email, model, "limited", log_dt, limit)

                complete_match = complete_regex.search(line)
                if complete_match:
                    request = pending_requests.pop(complete_match.group(1), None)
                    completed_at = _parse_log_timestamp(line, log_file)
                    if request and request["response"] and not request["failed"] and completed_at:
                        record_evidence(request["email"], request["model"], "available", completed_at)
        except OSError:
            pass

    detected_limits = {}
    for email, events in evidence.items():
        for event in events.values():
            if event["state"] == "limited":
                detected_limits.setdefault(email, {})[event["key"]] = event["limit"]

    _LOG_SCAN_CACHE = (signature, detected_limits, evidence)
    limits_copy = json.loads(json.dumps(detected_limits))
    evidence_copy = json.loads(json.dumps(evidence))
    return (limits_copy, evidence_copy) if include_evidence else limits_copy
