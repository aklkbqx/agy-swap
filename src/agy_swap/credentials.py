"""Platform Keychain (macOS, Windows, Linux) and local OAuth file operations."""

import platform
import subprocess

from agy_swap import OAUTH_FILE, OAUTH_CREDS_FILE, GOOGLE_ACCOUNTS_FILE
from agy_swap.oauth import decode_token
from agy_swap.storage import (
    _atomic_write_json, _snapshot_files, _restore_files, _session_lock,
)


def _windows_credential_api():
    import ctypes
    from ctypes import wintypes

    class Credential(ctypes.Structure):
        _fields_ = [
            ("Flags", wintypes.DWORD),
            ("Type", wintypes.DWORD),
            ("TargetName", wintypes.LPWSTR),
            ("Comment", wintypes.LPWSTR),
            ("LastWritten", wintypes.FILETIME),
            ("CredentialBlobSize", wintypes.DWORD),
            ("CredentialBlob", ctypes.POINTER(ctypes.c_ubyte)),
            ("Persist", wintypes.DWORD),
            ("AttributeCount", wintypes.DWORD),
            ("Attributes", ctypes.c_void_p),
            ("TargetAlias", wintypes.LPWSTR),
            ("UserName", wintypes.LPWSTR),
        ]

    return ctypes, wintypes, ctypes.windll.advapi32, Credential


def get_windows_credential():
    try:
        ctypes, wintypes, advapi32, credential_type = _windows_credential_api()
        pointer = ctypes.POINTER(credential_type)()
        if not advapi32.CredReadW(ctypes.c_wchar_p("gemini:antigravity"), wintypes.DWORD(1), wintypes.DWORD(0), ctypes.byref(pointer)):
            return None
        try:
            blob = ctypes.string_at(pointer.contents.CredentialBlob, pointer.contents.CredentialBlobSize)
            return blob.decode("utf-8").strip()
        finally:
            advapi32.CredFree(pointer)
    except (OSError, ValueError, UnicodeDecodeError):
        return None


def set_windows_credential(value):
    try:
        ctypes, wintypes, advapi32, credential_type = _windows_credential_api()
        encoded = value.encode("utf-8")
        blob = (ctypes.c_ubyte * len(encoded)).from_buffer_copy(encoded)
        credential = credential_type(
            Flags=0,
            Type=1,
            TargetName="gemini:antigravity",
            Comment=None,
            LastWritten=wintypes.FILETIME(),
            CredentialBlobSize=len(encoded),
            CredentialBlob=ctypes.cast(blob, ctypes.POINTER(ctypes.c_ubyte)),
            Persist=3,
            AttributeCount=0,
            Attributes=None,
            TargetAlias=None,
            UserName="antigravity",
        )
        return bool(advapi32.CredWriteW(ctypes.byref(credential), 0))
    except (OSError, ValueError):
        return False


def delete_windows_credential():
    try:
        ctypes, wintypes, advapi32, _ = _windows_credential_api()
        return bool(advapi32.CredDeleteW(ctypes.c_wchar_p("gemini:antigravity"), wintypes.DWORD(1), wintypes.DWORD(0)))
    except (OSError, ValueError):
        return False


def get_linux_credential():
    try:
        val = subprocess.check_output(
            ["secret-tool", "lookup", "service", "gemini", "username", "antigravity"],
            stderr=subprocess.DEVNULL,
            timeout=5,
        ).decode("utf-8").strip()
        return val
    except Exception:
        return None


def set_linux_credential(token_data_str):
    try:
        proc = subprocess.run(
            ["secret-tool", "store", "--label=gemini", "service", "gemini", "username", "antigravity"],
            input=token_data_str.encode("utf-8"),
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=10,
        )
        return proc.returncode == 0
    except Exception:
        return False


def delete_linux_credential():
    try:
        proc = subprocess.run(
            ["secret-tool", "clear", "service", "gemini", "username", "antigravity"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=5,
        )
        return proc.returncode == 0
    except Exception:
        return False


def _get_secure_token():
    plat = platform.system()
    if plat == "Darwin":
        try:
            val = subprocess.check_output(
                ["security", "find-generic-password", "-a", "antigravity", "-s", "gemini", "-w"],
                stderr=subprocess.DEVNULL,
                timeout=5,
            ).decode("utf-8").strip()
            return val
        except Exception:
            return None
    elif plat == "Windows":
        return get_windows_credential()
    elif plat == "Linux":
        return get_linux_credential()
    return None


def _read_oauth_file_token():
    import json, base64
    try:
        with open(OAUTH_FILE, "rb") as f:
            raw = f.read()
        json.loads(raw.decode("utf-8"))
        return "go-keyring-base64:" + base64.b64encode(raw).decode("utf-8")
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        return None


def get_current_keychain_token():
    return _get_secure_token() or _read_oauth_file_token()


def set_keychain_token(token_data_str):
    plat = platform.system()
    if plat == "Darwin":
        try:
            subprocess.run(
                ["security", "add-generic-password", "-U", "-a", "antigravity", "-s", "gemini", "-w", token_data_str],
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=10,
            )
            return True
        except (OSError, subprocess.SubprocessError):
            return False
    elif plat == "Windows":
        return set_windows_credential(token_data_str)
    elif plat == "Linux":
        return set_linux_credential(token_data_str)
    return False


def write_oauth_file(token_data_str, email=None):
    token_json = decode_token(token_data_str)
    inner = token_json.get("token") if isinstance(token_json, dict) else None
    if not isinstance(inner, dict) or not inner.get("access_token"):
        return False

    paths = [OAUTH_FILE, OAUTH_CREDS_FILE]
    if email:
        paths.append(GOOGLE_ACCOUNTS_FILE)
    snapshot = _snapshot_files(paths)
    try:
        creds = {
            "access_token": inner.get("access_token", ""),
            "refresh_token": inner.get("refresh_token", ""),
            "scope": inner.get("scope", "https://www.googleapis.com/auth/cloud-platform openid https://www.googleapis.com/auth/userinfo.email"),
            "token_type": inner.get("token_type", "Bearer"),
            "id_token": inner.get("id_token", ""),
            "expiry_date": inner.get("expiry_date", 0),
        }
        _atomic_write_json(OAUTH_FILE, token_json)
        _atomic_write_json(OAUTH_CREDS_FILE, creds)

        if email:
            import os, json
            ga = {"active": email, "old": []}
            try:
                if os.path.exists(GOOGLE_ACCOUNTS_FILE):
                    with open(GOOGLE_ACCOUNTS_FILE) as f:
                        existing = json.load(f)
                    old_active = existing.get("active", "")
                    old_list = existing.get("old", [])
                    if not isinstance(old_list, list):
                        old_list = []
                    if old_active and old_active != email and old_active not in old_list:
                        old_list = [old_active] + [x for x in old_list if x != email]
                    ga = {"active": email, "old": [x for x in old_list if x != email]}
            except (OSError, json.JSONDecodeError, AttributeError):
                pass
            _atomic_write_json(GOOGLE_ACCOUNTS_FILE, ga)

        return True
    except (OSError, TypeError, ValueError):
        _restore_files(snapshot)
        return False


def delete_keychain_token():
    plat = platform.system()
    if plat == "Darwin":
        try:
            subprocess.run(
                ["security", "delete-generic-password", "-a", "antigravity", "-s", "gemini"],
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                timeout=5,
            )
            return True
        except (OSError, subprocess.SubprocessError):
            return _get_secure_token() is None
    elif plat == "Windows":
        return delete_windows_credential()
    elif plat == "Linux":
        return delete_linux_credential()
    return False


def delete_oauth_files():
    import os
    ok = True
    for path in (OAUTH_FILE, OAUTH_CREDS_FILE, GOOGLE_ACCOUNTS_FILE):
        try:
            os.unlink(path)
        except FileNotFoundError:
            pass
        except OSError:
            ok = False
    return ok


def _apply_account_token_unlocked(token_data_str, email=None):
    if not decode_token(token_data_str):
        return False

    previous_secure = _get_secure_token()
    secure_updated = set_keychain_token(token_data_str)
    if not secure_updated:
        current_secure = _get_secure_token()
        if current_secure == token_data_str:
            secure_updated = True
        elif previous_secure:
            if current_secure != previous_secure:
                set_keychain_token(previous_secure)
            return False
        elif current_secure:
            return False

    if write_oauth_file(token_data_str, email=email):
        return True

    if secure_updated:
        if previous_secure:
            set_keychain_token(previous_secure)
        else:
            delete_keychain_token()
    return False


def apply_account_token(token_data_str, email=None):
    with _session_lock():
        return _apply_account_token_unlocked(token_data_str, email)


def _clear_active_session_unlocked():
    delete_keychain_token()
    delete_oauth_files()
    return _get_secure_token() is None and _read_oauth_file_token() is None


def clear_active_session():
    with _session_lock():
        return _clear_active_session_unlocked()


def enable_windows_ansi():
    if platform.system() == "Windows":
        try:
            import ctypes
            kernel32 = ctypes.windll.kernel32
            handle = kernel32.GetStdHandle(-11)
            mode = ctypes.c_ulong()
            kernel32.GetConsoleMode(handle, ctypes.byref(mode))
            kernel32.SetConsoleMode(handle, mode.value | 0x0004)
        except Exception:
            pass
