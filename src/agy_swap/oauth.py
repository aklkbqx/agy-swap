"""OAuth token decoding, Google API requests, and authentication claims."""

import base64
import json
import urllib.error
import urllib.parse
import urllib.request

from agy_swap import (
    DEFAULT_OAUTH_CLIENT_ID, OAUTH_CLIENT_SECRETS, OAUTH_TOKEN_URL, CLOUD_CODE_API,
    QuotaFetchError,
)
from agy_swap.display import normalize_email
from agy_swap.network import safe_urlopen


def decode_token(keychain_val):
    if not keychain_val or not keychain_val.startswith("go-keyring-base64:"):
        return None
    try:
        encoded = keychain_val.split(":", 1)[1]
        data = json.loads(base64.b64decode(encoded, validate=True).decode("utf-8"))
        return data if isinstance(data, dict) else None
    except (ValueError, UnicodeDecodeError, json.JSONDecodeError):
        return None


def extract_verified_google_email_claim(token_str):
    token_json = decode_token(token_str)
    token_obj = token_json.get("token") if token_json else None
    if not isinstance(token_obj, dict):
        return None
    id_token = token_obj.get("id_token") or token_json.get("id_token")
    if not id_token:
        return None
    try:
        parts = id_token.split(".")
        if len(parts) != 3:
            return None
        payload = parts[1] + "=" * (-len(parts[1]) % 4)
        claims = json.loads(base64.urlsafe_b64decode(payload).decode("utf-8"))
    except (ValueError, UnicodeDecodeError, json.JSONDecodeError):
        return None
    if claims.get("iss") not in ("accounts.google.com", "https://accounts.google.com"):
        return None
    if claims.get("email_verified") is not True:
        return None
    return normalize_email(claims.get("email"))


def _oauth_client_id(token_json):
    token = token_json.get("token", {})
    id_token = token.get("id_token") or token_json.get("id_token")
    try:
        payload = id_token.split(".")[1]
        payload += "=" * (-len(payload) % 4)
        audience = json.loads(base64.urlsafe_b64decode(payload).decode("utf-8")).get("aud")
    except (AttributeError, IndexError, ValueError, UnicodeDecodeError, json.JSONDecodeError):
        audience = None
    return audience if audience in OAUTH_CLIENT_SECRETS else DEFAULT_OAUTH_CLIENT_ID


def _refresh_access_token(token_data):
    token_json = decode_token(token_data)
    token = token_json.get("token") if token_json else None
    refresh_token = token.get("refresh_token") if isinstance(token, dict) else None
    if not refresh_token:
        raise QuotaFetchError("refresh token is unavailable")
    client_id = _oauth_client_id(token_json)
    body = urllib.parse.urlencode({
        "client_id": client_id,
        "client_secret": OAUTH_CLIENT_SECRETS[client_id],
        "refresh_token": refresh_token,
        "grant_type": "refresh_token",
    }).encode("ascii")
    request = urllib.request.Request(OAUTH_TOKEN_URL, data=body, method="POST")
    try:
        with safe_urlopen(request, timeout=15) as response:
            access_token = json.loads(response.read().decode("utf-8")).get("access_token")
    except urllib.error.HTTPError as exc:
        exc.close()
        raise QuotaFetchError(f"OAuth refresh failed (HTTP {exc.code})") from None
    except (OSError, ValueError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise QuotaFetchError(f"OAuth refresh failed: {exc}") from None
    if not access_token:
        raise QuotaFetchError("OAuth refresh returned no access token")
    return access_token


def _cloud_code_post(access_token, method, body):
    request = urllib.request.Request(
        CLOUD_CODE_API + method,
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {access_token}",
            "Content-Type": "application/json",
            "User-Agent": "antigravity",
        },
        method="POST",
    )
    try:
        with safe_urlopen(request, timeout=15) as response:
            result = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        exc.close()
        raise QuotaFetchError(f"{method} failed (HTTP {exc.code})") from None
    except (OSError, ValueError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise QuotaFetchError(f"{method} failed: {exc}") from None
    if not isinstance(result, dict):
        raise QuotaFetchError(f"{method} returned invalid data")
    return result


def get_google_userinfo(access_token):
    url = "https://www.googleapis.com/oauth2/v1/userinfo"
    try:
        req = urllib.request.Request(url, headers={"Authorization": f"Bearer {access_token}"})
        with safe_urlopen(req, timeout=5) as response:
            if response.status == 200:
                data = json.loads(response.read().decode("utf-8"))
                return data
    except urllib.error.HTTPError as exc:
        exc.close()
    except (OSError, ValueError, UnicodeDecodeError, json.JSONDecodeError):
        pass
    return None
