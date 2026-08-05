import base64
from contextlib import nullcontext, redirect_stdout
import io
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from datetime import datetime, timedelta, timezone


SCRIPT = Path(__file__).resolve().parents[1] / "agy-swap"


def load_script():
    namespace = {"__name__": "agy_swap_test", "__file__": str(SCRIPT)}
    exec(compile(SCRIPT.read_text(encoding="utf-8"), str(SCRIPT), "exec"), namespace)
    return namespace


def token_blob(email="user@example.com", refresh_token="refresh"):
    header = base64.urlsafe_b64encode(b'{"alg":"none"}').decode().rstrip("=")
    claim_data = {
        "iss": "https://accounts.google.com",
        "email_verified": True,
    }
    if email is not None:
        claim_data["email"] = email
    claims = base64.urlsafe_b64encode(json.dumps(claim_data).encode()).decode().rstrip("=")
    token = {
        "token": {
            "access_token": "access",
            "refresh_token": refresh_token,
            "id_token": f"{header}.{claims}.signature",
        }
    }
    return "go-keyring-base64:" + base64.b64encode(json.dumps(token).encode()).decode()


class AgySwapTests(unittest.TestCase):
    def setUp(self):
        self.m = load_script()

    def test_duration_parser_is_strict_and_supports_days(self):
        parse = self.m["parse_duration_seconds"]
        self.assertEqual(parse("4h 30m"), 16200)
        self.assertEqual(parse("6d"), 518400)
        self.assertEqual(parse("reset"), 0)
        self.assertIsNone(parse("abc1hxyz"))
        self.assertIsNone(parse("8d"))
        self.assertEqual(self.m["format_duration"](6 * 86400 + 2 * 3600 + 7 * 60), "6d 2h 7m")

    def test_next_account_skips_limits_and_uses_shortest_fallback(self):
        now = datetime.now(timezone.utc)
        accounts = {
            "active@example.com": {"email": "active@example.com", "name": "Active"},
            "long@example.com": {
                "email": "long@example.com", "name": "Long",
                "quota_limits": {
                    "claude-opus": {
                        "model": "Claude Opus",
                        "family": "claude",
                        "reset_at": (now + timedelta(hours=3)).isoformat(),
                        "observed_at": now.isoformat(),
                        "source": "log",
                    }
                },
            },
            "ready@example.com": {"email": "ready@example.com", "name": "Ready"},
        }
        selected, all_limited = self.m["select_next_account"](accounts, "active@example.com")
        self.assertEqual(selected["email"], "ready@example.com")
        self.assertIsNone(all_limited)

        accounts["ready@example.com"]["quota_limits"] = {
            "gemini-flash": {
                "model": "Gemini Flash",
                "family": "gemini",
                "reset_at": (now + timedelta(minutes=20)).isoformat(),
                "observed_at": now.isoformat(),
                "source": "log",
            }
        }
        accounts["active@example.com"]["quota_limits"] = {
            "claude-sonnet": {
                "model": "Claude Sonnet",
                "family": "claude",
                "reset_at": (now + timedelta(hours=1)).isoformat(),
                "observed_at": now.isoformat(),
                "source": "log",
            }
        }
        selected, all_limited = self.m["select_next_account"](accounts, "active@example.com")
        self.assertEqual(selected["email"], "ready@example.com")
        self.assertTrue(all_limited)

    def test_quota_scan_parses_real_glog_prefix_and_keeps_exact_model(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as log:
            now = datetime.now().astimezone()
            prefix = now.strftime("%m%d %H:%M:%S")
            log.write(f"ERROR: logging before google.Init: I{prefix}.000001 1 server_oauth.go:189] applyAuthResult: email=user@example.com, authMethod=consumer\n")
            log.write(f'ERROR: logging before google.Init: I{prefix}.000002 1 model_config_manager.go:272] Propagating selected model override to backend: label="Claude Opus 4.6 (Thinking)"\n')
            log.write(f"ERROR: logging before google.Init: E{prefix}.000003 1 stream_handler.go:101] RESOURCE_EXHAUSTED (code 429): Resets in 6d\n")
            log_path = log.name
        try:
            os.utime(log_path, (now.timestamp(), now.timestamp()))
            self.m["find_antigravity_logs"] = lambda: [log_path]
            self.m["_LOG_SCAN_CACHE"] = None
            result = self.m["auto_scan_logs_for_limits"]()
            limits = list(result["user@example.com"].values())
            self.assertEqual(len(limits), 1)
            self.assertEqual(limits[0]["model"], "Claude Opus 4.6 (Thinking)")
            self.assertEqual(limits[0]["family"], "claude")
            self.assertEqual(limits[0]["source"], "log")
            self.assertEqual(limits[0]["source_file"], os.path.basename(log_path))
        finally:
            os.unlink(log_path)

    def test_quota_scan_ignores_error_without_explicit_model_context(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as log:
            stamp = datetime.now().astimezone().strftime("%Y-%m-%d %H:%M:%S")
            log.write("applyAuthResult: email=user@example.com\n")
            log.write(f"{stamp} RESOURCE_EXHAUSTED Resets in 4h\n")
            log_path = log.name
        try:
            self.m["find_antigravity_logs"] = lambda: [log_path]
            self.m["_LOG_SCAN_CACHE"] = None
            self.assertEqual(self.m["auto_scan_logs_for_limits"](), {})
        finally:
            os.unlink(log_path)

    def test_newer_confirmed_success_clears_an_older_cooldown(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as log:
            now = datetime.now().astimezone().replace(microsecond=0)
            older = now - timedelta(seconds=10)
            conversation = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
            log.write(f"I{older.strftime('%m%d %H:%M:%S')} applyAuthResult: email=user@example.com, authMethod=consumer\n")
            log.write(f'I{older.strftime("%m%d %H:%M:%S")} Propagating selected model override to backend: label="Claude Opus 4.6 (Thinking)"\n')
            log.write(f"E{older.strftime('%m%d %H:%M:%S')} RESOURCE_EXHAUSTED (code 429): Resets in 6d\n")
            log.write(f'I{now.strftime("%m%d %H:%M:%S")} Propagating selected model override to backend: label="Claude Opus 4.6 (Thinking)"\n')
            log.write(f"I{now.strftime('%m%d %H:%M:%S')} Sending user message to conversation {conversation}\n")
            log.write(f"I{now.strftime('%m%d %H:%M:%S')} streamGenerateContent ResponseID: success\n")
            log.write(f"I{now.strftime('%m%d %H:%M:%S')} Stream completed for {conversation}, clearing ResponsePending\n")
            log_path = log.name
        try:
            os.utime(log_path, (now.timestamp(), now.timestamp()))
            self.m["find_antigravity_logs"] = lambda: [log_path]
            self.m["_LOG_SCAN_CACHE"] = None
            limits, evidence = self.m["auto_scan_logs_for_limits"](include_evidence=True)
            self.assertEqual(limits, {})
            self.assertEqual(next(iter(evidence["user@example.com"].values()))["state"], "available")
        finally:
            os.unlink(log_path)

    def test_stream_completion_does_not_clear_a_failed_request(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as log:
            now = datetime.now().astimezone().replace(microsecond=0)
            conversation = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
            stamp = now.strftime("%m%d %H:%M:%S")
            log.write(f"I{stamp} applyAuthResult: email=user@example.com, authMethod=consumer\n")
            log.write(f'I{stamp} Propagating selected model override to backend: label="Gemini 3.6 Flash (High)"\n')
            log.write(f"I{stamp} Sending user message to conversation {conversation}\n")
            log.write(f"I{stamp} streamGenerateContent ResponseID: quota-response\n")
            log.write(f"E{stamp} RESOURCE_EXHAUSTED (code 429): Resets in 4h\n")
            log.write(f"I{stamp} Stream completed for {conversation}, clearing ResponsePending\n")
            log_path = log.name
        try:
            os.utime(log_path, (now.timestamp(), now.timestamp()))
            self.m["find_antigravity_logs"] = lambda: [log_path]
            self.m["_LOG_SCAN_CACHE"] = None
            limits, evidence = self.m["auto_scan_logs_for_limits"](include_evidence=True)
            self.assertEqual(len(limits["user@example.com"]), 1)
            self.assertEqual(next(iter(evidence["user@example.com"].values()))["state"], "limited")
        finally:
            os.unlink(log_path)

    def test_resolving_model_alias_uses_one_canonical_limit(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as log:
            now = datetime.now().astimezone().replace(microsecond=0)
            stamp = now.strftime("%m%d %H:%M:%S")
            log.write(f"I{stamp} applyAuthResult: email=user@example.com, authMethod=consumer\n")
            log.write(f"I{stamp} Resolving model claude-sonnet-4-6\n")
            log.write(f'I{stamp} Propagating selected model override to backend: label="Claude Sonnet 4.6 (Thinking)"\n')
            log.write(f"I{stamp} Resolving model claude-sonnet-4-6\n")
            log.write(f"E{stamp} RESOURCE_EXHAUSTED (code 429): Resets in 5d\n")
            log_path = log.name
        try:
            os.utime(log_path, (now.timestamp(), now.timestamp()))
            self.m["find_antigravity_logs"] = lambda: [log_path]
            self.m["_LOG_SCAN_CACHE"] = None
            limits = self.m["auto_scan_logs_for_limits"]()["user@example.com"]
            self.assertEqual(list(limits), ["claude-sonnet-4-6"])
            self.assertEqual(limits["claude-sonnet-4-6"]["model"], "Claude Sonnet 4.6 (Thinking)")
        finally:
            os.unlink(log_path)

    def test_unknown_quota_is_not_rendered_as_ready_or_full(self):
        display = self.m["get_account_limit_display"]({"email": "user@example.com"})
        self.assertIn("Unknown", display)
        self.assertIn("Usage unavailable", display)
        self.assertNotIn("Ready", display)
        self.assertNotIn("100%", display)

    def _fetch_api_quota(self, email, account_info, groups):
        self.m["_refresh_access_token"] = lambda _token: "fresh-access"
        self.m["get_google_userinfo"] = lambda _token: {"email": email}
        self.m["_cloud_code_post"] = lambda _token, method, _body: (
            account_info if method == "loadCodeAssist" else {"groups": groups}
        )
        return self.m["fetch_account_quota"]({"email": email, "token_data": token_blob(email)})

    def test_paid_quota_prefers_paid_tier_and_keeps_weekly_and_five_hour(self):
        reset = (datetime.now(timezone.utc) + timedelta(days=1)).isoformat()
        snapshot = self._fetch_api_quota(
            "paid@example.com",
            {
                "cloudaicompanionProject": "project",
                "currentTier": {"id": "free-tier", "name": "Free"},
                "paidTier": {"id": "g1-pro-tier", "name": "Pro"},
            },
            [{
                "displayName": "Gemini Models",
                "buckets": [
                    {"bucketId": "gemini-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": reset, "remainingFraction": 0.75},
                    {"bucketId": "gemini-5h", "displayName": "5-hour Limit", "window": "5h", "resetTime": reset, "remainingFraction": 0.5},
                ],
            }],
        )
        self.assertEqual(snapshot["tier"]["id"], "g1-pro-tier")
        self.assertEqual([bucket["window"] for bucket in snapshot["groups"][0]["buckets"]], ["weekly", "5h"])

    def test_free_quota_does_not_fabricate_five_hour_bucket(self):
        reset = (datetime.now(timezone.utc) + timedelta(days=1)).isoformat()
        snapshot = self._fetch_api_quota(
            "free@example.com",
            {"cloudaicompanionProject": "project", "currentTier": {"id": "free-tier", "name": "Free"}},
            [{
                "displayName": "Gemini Models",
                "buckets": [{"bucketId": "gemini-weekly", "displayName": "Weekly Limit", "window": "weekly", "resetTime": reset, "remainingFraction": 0.25}],
            }],
        )
        account = {"quota_snapshot": snapshot}
        self.assertEqual(snapshot["tier"]["id"], "free-tier")
        self.assertEqual([bucket["window"] for bucket in snapshot["groups"][0]["buckets"]], ["weekly"])
        self.assertNotIn("5-hour", " ".join(bucket["name"] for group in self.m["quota_groups"](account) for bucket in group["buckets"]))

    def test_api_quota_routes_gemini_and_third_party_independently(self):
        now = datetime.now(timezone.utc)
        reset = (now + timedelta(hours=2)).isoformat()
        limited = {
            "email": "limited@example.com",
            "quota_snapshot": {
                "observed_at": now.isoformat(),
                "tier": {"id": "g1-pro-tier", "name": "Google AI Pro"},
                "groups": [
                    {"id": "gemini", "name": "Gemini Models", "buckets": [{"id": "gemini-weekly", "name": "Weekly Limit", "window": "weekly", "remaining_fraction": 0.0, "reset_at": reset}]},
                    {"id": "third_party", "name": "Claude and GPT", "buckets": [{"id": "3p-weekly", "name": "Weekly Limit", "window": "weekly", "remaining_fraction": 0.5, "reset_at": reset}]},
                ],
            },
        }
        ready = {"email": "ready@example.com"}
        accounts = {limited["email"]: limited, ready["email"]: ready}
        selected, _ = self.m["select_next_account"](accounts, ready["email"], "gemini")
        self.assertEqual(selected["email"], ready["email"])
        selected, all_limited = self.m["select_next_account"](accounts, ready["email"], "claude")
        self.assertEqual(selected["email"], limited["email"])
        self.assertFalse(all_limited)

    def test_failed_quota_refresh_keeps_cached_snapshot_and_reports_failure(self):
        observed = datetime.now(timezone.utc) - timedelta(hours=1)
        reset = (observed + timedelta(days=1)).isoformat()
        snapshot = {
            "observed_at": observed.isoformat(),
            "tier": {"id": "free-tier", "name": "Free"},
            "groups": [{"id": "gemini", "name": "Gemini Models", "buckets": [{"id": "gemini-weekly", "name": "Weekly Limit", "window": "weekly", "remaining_fraction": 0.42, "reset_at": reset}]}],
        }
        accounts = self.m["Accounts"]({"user@example.com": {"email": "user@example.com", "quota_snapshot": snapshot}})
        statuses = []

        def fail(_account):
            raise self.m["QuotaFetchError"]("offline")

        self.m["fetch_account_quota"] = fail
        errors = self.m["refresh_quota_snapshots"](accounts, force=True, progress=lambda _i, _t, _a, status: statuses.append(status))
        self.assertEqual(errors, {"user@example.com": "offline"})
        self.assertEqual(accounts["user@example.com"]["quota_snapshot"], snapshot)
        self.assertEqual(statuses, ["failed"])
        display = self.m["get_account_limit_display"](accounts["user@example.com"])
        self.assertNotIn("Ready", display)
        self.assertNotIn("100%", display)

    def test_oauth_client_id_falls_back_when_id_token_is_missing(self):
        self.assertEqual(
            self.m["_oauth_client_id"]({"token": {"refresh_token": "refresh"}}),
            self.m["DEFAULT_OAUTH_CLIENT_ID"],
        )

    def test_cooldown_bar_is_time_based_not_quota_based(self):
        now = datetime.now(timezone.utc)
        limit = {
            "observed_at": (now - timedelta(hours=1)).isoformat(),
            "reset_at": (now + timedelta(hours=1)).isoformat(),
        }
        progress = self.m["format_cooldown_bar"](limit, now=now, width=10)
        self.assertEqual(progress.count("█"), 5)
        self.assertIn("50.0% time left", progress)
        self.assertNotIn("quota", progress.lower())

    def test_no_cooldown_bar_is_explicit_and_not_a_quota_claim(self):
        progress = self.m["format_cooldown_bar"](None, width=10)
        self.assertEqual(progress.count("░"), 10)
        self.assertIn("No active cooldown observed", progress)
        self.assertNotIn("%", progress)
        self.assertNotIn("quota", progress.lower())

    def test_reconcile_deduplicates_log_aliases_and_preserves_manual_limit(self):
        now = datetime.now(timezone.utc)
        old = (now - timedelta(minutes=1)).isoformat()
        reset = (now + timedelta(hours=2)).isoformat()
        account = {
            "quota_limits": {
                "claude-sonnet-4-6": {
                    "model": "claude-sonnet-4-6", "family": "claude",
                    "observed_at": old, "reset_at": reset, "source": "log",
                },
                "claude-sonnet-4-6-thinking": {
                    "model": "Claude Sonnet 4.6 (Thinking)", "family": "claude",
                    "observed_at": old, "reset_at": reset, "source": "log",
                },
                "manual:gpt": {
                    "model": "GPT", "family": "gpt",
                    "observed_at": old, "reset_at": reset, "source": "manual",
                },
            }
        }
        fresh = {
            "model": "Claude Sonnet 4.6 (Thinking)", "family": "claude",
            "observed_at": now.isoformat(), "reset_at": reset, "source": "log",
        }
        evidence = {
            "claude-sonnet-4-6": {
                "state": "limited", "observed_at": now.isoformat(),
                "model": fresh["model"], "family": "claude", "key": "claude-sonnet-4-6",
            }
        }
        self.assertTrue(self.m["reconcile_log_limits"](account, {"claude-sonnet-4-6": fresh}, evidence))
        self.assertEqual(set(account["quota_limits"]), {"claude-sonnet-4-6", "manual:gpt"})
        self.assertEqual(account["quota_limits"]["claude-sonnet-4-6"]["model"], fresh["model"])

    def test_legacy_quota_is_preserved_but_removed_from_active_fields(self):
        account = {
            "limit_reset_claude": "2026-08-07T14:35:10+00:00",
            "claude_pct": 0,
            "limit_reset_gemini": "2026-08-07T14:35:10+00:00",
            "gemini_pct": 0,
        }
        self.assertTrue(self.m["migrate_legacy_quota"](account))
        self.assertEqual(account["quota_schema"], 2)
        self.assertIn("limit_reset_claude", account["legacy_quota"])
        self.assertNotIn("limit_reset_claude", account)
        self.assertNotIn("gemini_pct", account)

    def test_load_migrates_legacy_quota_with_backup_and_fresh_scan(self):
        with tempfile.TemporaryDirectory() as base:
            config = Path(base)
            accounts_file = config / "accounts.json"
            original = {
                "user@example.com": {
                    "email": "user@example.com",
                    "name": "User",
                    "token_data": token_blob(),
                    "limit_reset_claude": "2026-08-07T14:35:10+00:00",
                    "claude_pct": 0,
                }
            }
            accounts_file.write_text(json.dumps(original), encoding="utf-8")
            self.m["CONFIG_DIR"] = str(config)
            self.m["ACCOUNTS_FILE"] = str(accounts_file)
            self.m["ACCOUNTS_LOCK_FILE"] = str(config / ".accounts.lock")
            now = datetime.now(timezone.utc)
            detected = {
                "user@example.com": {
                    "claude-opus": {
                        "model": "Claude Opus",
                        "family": "claude",
                        "reset_at": (now + timedelta(hours=2)).isoformat(),
                        "observed_at": now.isoformat(),
                        "source": "log",
                    }
                }
            }
            self.m["auto_scan_logs_for_limits"] = lambda include_evidence=False: (detected, {}) if include_evidence else detected

            accounts = self.m["load_accounts"]()

            self.assertIn("limit_reset_claude", accounts["user@example.com"]["legacy_quota"])
            self.assertEqual(accounts["user@example.com"]["quota_limits"]["claude-opus"]["source"], "log")
            backup = json.loads((config / "accounts.json.bak").read_text(encoding="utf-8"))
            self.assertIn("limit_reset_claude", backup["user@example.com"])

    def test_account_store_is_owner_only_atomic_and_fail_closed(self):
        with tempfile.TemporaryDirectory() as base:
            config = Path(base) / "config"
            self.m["CONFIG_DIR"] = str(config)
            self.m["ACCOUNTS_FILE"] = str(config / "accounts.json")
            self.m["ACCOUNTS_LOCK_FILE"] = str(config / ".accounts.lock")
            accounts = self.m["Accounts"]({
                "user@example.com": {
                    "email": "user@example.com",
                    "name": "User",
                    "token_data": token_blob(),
                }
            })
            accounts.revision = None
            self.m["save_accounts"](accounts)
            if os.name != "nt":
                self.assertEqual(config.stat().st_mode & 0o777, 0o700)
                self.assertEqual((config / "accounts.json").stat().st_mode & 0o777, 0o600)
            self.assertEqual(list(self.m["load_accounts"](sync_logs=False)), ["user@example.com"])

            current = self.m["load_accounts"](sync_logs=False)
            stale = self.m["load_accounts"](sync_logs=False)
            current["user@example.com"]["name"] = "Current"
            self.m["save_accounts"](current)
            stale["user@example.com"]["name"] = "Stale"
            with self.assertRaises(self.m["AccountStoreError"]):
                self.m["save_accounts"](stale)

            (config / "accounts.json").write_text("[{}]", encoding="utf-8")
            with self.assertRaises(self.m["AccountStoreError"]):
                self.m["load_accounts"](sync_logs=False)

    def test_nested_jwt_email_is_read_without_persisting_token(self):
        self.assertEqual(self.m["extract_email_from_token"](token_blob()), "user@example.com")

    def test_verified_token_email_mismatch_is_rejected_but_missing_claim_is_allowed(self):
        with self.assertRaises(self.m["AccountStoreError"]):
            self.m["_validate_accounts"]({
                "alice@example.com": {
                    "email": "alice@example.com",
                    "token_data": token_blob("bob@example.com"),
                }
            })
        accounts = self.m["_validate_accounts"]({
            "alice@example.com": {
                "email": "alice@example.com",
                "token_data": token_blob(None),
            }
        })
        self.assertIn("alice@example.com", accounts)

    def test_failed_secure_write_and_oauth_write_restore_previous_token(self):
        previous = token_blob("previous@example.com")
        requested = token_blob("requested@example.com")
        state = {"token": previous}

        def set_token(value):
            state["token"] = value
            return value != requested

        self.m["_get_secure_token"] = lambda: state["token"]
        self.m["set_keychain_token"] = set_token
        self.m["write_oauth_file"] = lambda *_args, **_kwargs: False

        self.assertFalse(self.m["_apply_account_token_unlocked"](requested, "requested@example.com"))
        self.assertEqual(state["token"], previous)

    def test_token_stdin_has_a_hard_size_limit(self):
        previous_stdin = self.m["sys"].stdin
        try:
            self.m["sys"].stdin = io.StringIO("x" * (self.m["MAX_TOKEN_BYTES"] + 1))
            with self.assertRaises(ValueError):
                self.m["_read_token_stdin"]()
        finally:
            self.m["sys"].stdin = previous_stdin

    def test_windows_special_keys_are_normalized(self):
        cases = (
            ((b"\xe0", b"H"), "up"),
            ((b"\xe0", b"P"), "down"),
            ((b"\xe0", b"S"), "delete"),
            ((b"\x1b",), "esc"),
            ((b"\x08",), "backspace"),
            ((b"\r",), "\n"),
        )
        for sequence, expected in cases:
            keys = iter(sequence)
            self.assertEqual(self.m["_read_windows_key"](lambda: next(keys)), expected)

    def test_next_family_ignores_other_model_cooldowns(self):
        now = datetime.now(timezone.utc)
        accounts = {
            "active@example.com": {"email": "active@example.com"},
            "gemini@example.com": {
                "email": "gemini@example.com",
                "quota_limits": {
                    "gemini": {
                        "model": "Gemini Flash", "family": "gemini",
                        "reset_at": (now + timedelta(hours=3)).isoformat(),
                        "observed_at": now.isoformat(), "source": "log",
                    }
                },
            },
            "claude@example.com": {
                "email": "claude@example.com",
                "quota_limits": {
                    "claude": {
                        "model": "Claude Opus", "family": "claude",
                        "reset_at": (now + timedelta(minutes=5)).isoformat(),
                        "observed_at": now.isoformat(), "source": "log",
                    }
                },
            },
        }
        selected, all_limited = self.m["select_next_account"](accounts, "active@example.com", "claude")
        self.assertEqual(selected["email"], "gemini@example.com")
        self.assertIsNone(all_limited)

    def test_all_limited_next_message_does_not_claim_no_cooldown(self):
        now = datetime.now(timezone.utc)
        accounts = {}
        for index, minutes in enumerate((30, 10), 1):
            email = f"user{index}@example.com"
            accounts[email] = {
                "email": email, "name": f"User {index}", "token_data": token_blob(email),
                "quota_limits": {
                    "claude": {
                        "model": "Claude Opus", "family": "claude",
                        "reset_at": (now + timedelta(minutes=minutes)).isoformat(),
                        "observed_at": now.isoformat(), "source": "log",
                    }
                },
            }
        self.m["_session_lock"] = nullcontext
        self.m["load_accounts"] = lambda: accounts
        self.m["refresh_quota_snapshots"] = lambda *_args, **_kwargs: {}
        self.m["get_current_keychain_token"] = lambda: accounts["user1@example.com"]["token_data"]
        self.m["_apply_account_token_unlocked"] = lambda *_args, **_kwargs: True
        output = io.StringIO()
        with redirect_stdout(output):
            self.m["cmd_next"](SimpleNamespace(family="claude"))
        self.assertIn("shortest observed Claude limit", output.getvalue())
        self.assertNotIn("no observed cooldown", output.getvalue())

    def test_quota_provenance_is_reduced_to_a_basename(self):
        now = datetime.now(timezone.utc)
        accounts = self.m["_validate_accounts"]({
            "user@example.com": {
                "email": "user@example.com",
                "quota_limits": {
                    "claude": {
                        "model": "Claude Opus", "family": "claude",
                        "reset_at": (now + timedelta(hours=1)).isoformat(),
                        "observed_at": now.isoformat(), "source": "log",
                        "source_file": "/private/secret/cli.log",
                    }
                },
            }
        })
        self.assertEqual(accounts["user@example.com"]["quota_limits"]["claude"]["source_file"], "cli.log")

    def test_ambiguous_account_target_is_rejected(self):
        accounts = {"one@example.com": {}, "two@example.com": {}}
        with self.assertRaises(self.m["AmbiguousAccountError"]):
            self.m["resolve_account_target"]("example", accounts)

    def test_clear_active_session_removes_all_oauth_files(self):
        with tempfile.TemporaryDirectory() as base:
            paths = [Path(base) / name for name in ("token", "creds", "accounts")]
            for path in paths:
                path.write_text("{}", encoding="utf-8")
            self.m["OAUTH_FILE"], self.m["OAUTH_CREDS_FILE"], self.m["GOOGLE_ACCOUNTS_FILE"] = map(str, paths)
            self.m["CONFIG_DIR"] = base
            self.m["SESSION_LOCK_FILE"] = str(Path(base) / ".session.lock")
            self.m["delete_keychain_token"] = lambda: True
            self.m["_get_secure_token"] = lambda: None
            self.assertTrue(self.m["clear_active_session"]())
            self.assertFalse(any(path.exists() for path in paths))

    def test_oauth_file_write_rolls_back_as_a_group(self):
        with tempfile.TemporaryDirectory() as base:
            paths = [Path(base) / name for name in ("token", "creds", "accounts")]
            for path in paths:
                path.write_bytes(b"before")
            self.m["OAUTH_FILE"], self.m["OAUTH_CREDS_FILE"], self.m["GOOGLE_ACCOUNTS_FILE"] = map(str, paths)
            atomic_write_json = self.m["_atomic_write_json"]

            def fail_on_creds(path, data):
                if path == str(paths[1]):
                    raise OSError("simulated failure")
                atomic_write_json(path, data)

            self.m["_atomic_write_json"] = fail_on_creds
            self.assertFalse(self.m["write_oauth_file"](token_blob(), "user@example.com"))
            self.assertTrue(all(path.read_bytes() == b"before" for path in paths))

    def test_terminal_width_helpers_handle_wide_characters(self):
        self.assertEqual(self.m["visible_len"]("Aก"), 2)
        self.assertLessEqual(self.m["visible_len"](self.m["truncate_visible"]("abcdef", 4)), 4)

    def test_non_tty_avatar_has_no_ansi_escape(self):
        self.assertNotIn("\x1b", self.m["get_avatar_badge"]("Google User", "user@example.com"))

    def test_unix_escape_sequences_do_not_become_escape(self):
        for sequence, expected in ((b"\x1b[A", "up"), (b"\x1b[B", "down"), (b"\x1b[3~", "delete")):
            read_fd, write_fd = os.pipe()
            try:
                os.write(write_fd, sequence)
                self.assertEqual(self.m["_read_unix_key"](read_fd), expected)
            finally:
                os.close(read_fd)
                os.close(write_fd)

    def test_safe_urlopen_fallbacks_on_ssl_verification_error(self):
        calls = []

        def mock_urlopen(req, timeout=10, context=None):
            calls.append(context)
            if len(calls) == 1:
                import ssl
                raise self.m["urllib"].error.URLError(ssl.SSLCertVerificationError("cert failed"))
            response = SimpleNamespace(read=lambda: b"success", status=200)
            return response

        original_urlopen = self.m["urllib"].request.urlopen
        try:
            self.m["urllib"].request.urlopen = mock_urlopen
            resp = self.m["safe_urlopen"]("https://example.com")
            self.assertEqual(resp.read(), b"success")
            self.assertEqual(len(calls), 2)
        finally:
            self.m["urllib"].request.urlopen = original_urlopen


if __name__ == "__main__":
    unittest.main()
