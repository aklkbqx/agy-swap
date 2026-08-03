"""Google Antigravity CLI Account Switcher and Quota Monitor."""

import os
import re

VERSION = "1.8.0"

# ── Paths ──
CONFIG_DIR = os.path.expanduser("~/.gemini/agy-swap")
ACCOUNTS_FILE = os.path.join(CONFIG_DIR, "accounts.json")
OAUTH_FILE = os.path.expanduser("~/.gemini/antigravity-cli/antigravity-oauth-token")
OAUTH_CREDS_FILE = os.path.expanduser("~/.gemini/oauth_creds.json")
GOOGLE_ACCOUNTS_FILE = os.path.expanduser("~/.gemini/google_accounts.json")
ACCOUNTS_LOCK_FILE = os.path.join(CONFIG_DIR, ".accounts.lock")
SESSION_LOCK_FILE = os.path.join(CONFIG_DIR, ".session.lock")

# ── Limits & Timeouts ──
MAX_LIMIT_SECS = 7 * 24 * 3600
MAX_TOKEN_BYTES = 1024 * 1024
LOG_SCAN_BYTES = 8 * 1024 * 1024
LOG_TOTAL_SCAN_BYTES = 64 * 1024 * 1024
QUOTA_SCHEMA = 2
QUOTA_CACHE_SECS = 60
TUI_AUTO_REFRESH_SECS = 60

# ── API URLs & Repos ──
CLOUD_CODE_API = "https://daily-cloudcode-pa.googleapis.com/v1internal:"
OAUTH_TOKEN_URL = "https://oauth2.googleapis.com/token"
GITHUB_REPO = "aklkbqx/agy-swap"
GITHUB_API_RELEASE = f"https://api.github.com/repos/{GITHUB_REPO}/releases/latest"
GITHUB_RAW = f"https://raw.githubusercontent.com/{GITHUB_REPO}"

# ── OAuth Client Secrets ──
DEFAULT_OAUTH_CLIENT_ID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
OAUTH_CLIENT_SECRETS = {
    DEFAULT_OAUTH_CLIENT_ID: "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",
    "884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com": "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",
}

# ── Tier Name Display Map ──
TIER_NAMES = {
    "free-tier": "Free",
    "g1-pro-tier": "Google AI Pro",
    "g1-ultra-tier": "Google AI Ultra",
    "g1-ultra-lite-tier": "Google AI Ultra Lite",
}

# ── ANSI Color Codes ──
ORANGE = "\033[38;5;208m"
GREEN = "\033[38;5;78m"
BLUE = "\033[38;5;75m"
RED = "\033[38;5;203m"
YELLOW = "\033[38;5;220m"
CYAN = "\033[38;5;86m"
GRAY = "\033[38;5;244m"
DARK_GRAY = "\033[38;5;238m"
BRIGHT_WHITE = "\033[38;5;255m"
BOLD = "\033[1m"
RESET = "\033[0m"
ANSI_ESCAPE = re.compile(r"\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])")

# ── Exceptions ──
class AccountStoreError(RuntimeError):
    pass

class AmbiguousAccountError(ValueError):
    pass

class QuotaFetchError(RuntimeError):
    pass

# ── Custom Data Types ──
class Accounts(dict):
    revision = None

# ── Global State Caches ──
_LOG_SCAN_CACHE = None
