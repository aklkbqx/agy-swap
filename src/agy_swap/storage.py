"""Atomic file operations and file-based locking."""

from contextlib import contextmanager
import json
import os
import platform
import tempfile

from agy_swap import CONFIG_DIR, ACCOUNTS_LOCK_FILE, SESSION_LOCK_FILE


def _set_private_mode(path, mode):
    # Windows Credential Manager protects the live OAuth token. For the local
    # account store, keep the normal inherited Windows ACL instead of applying
    # POSIX mode bits, which can make the directory unreadable to the next
    # Python process on Windows.
    if platform.system() != "Windows":
        os.chmod(path, mode)


def _directory_mode():
    return 0o777 if platform.system() == "Windows" else 0o700


def _atomic_write_bytes(path, data):
    directory = os.path.dirname(path)
    os.makedirs(directory, mode=_directory_mode(), exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".tmp.", dir=directory)
    try:
        if platform.system() != "Windows" and hasattr(os, "fchmod"):
            os.fchmod(fd, 0o600)
        with os.fdopen(fd, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp, path)
        _set_private_mode(path, 0o600)
    except Exception:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise


def _atomic_write_json(path, data):
    _atomic_write_bytes(path, json.dumps(data, ensure_ascii=False).encode("utf-8"))


def _snapshot_files(paths):
    snapshot = {}
    for path in paths:
        try:
            with open(path, "rb") as f:
                snapshot[path] = f.read()
        except FileNotFoundError:
            snapshot[path] = None
    return snapshot


def _restore_files(snapshot):
    ok = True
    for path, data in snapshot.items():
        try:
            if data is None:
                os.unlink(path)
            else:
                _atomic_write_bytes(path, data)
        except FileNotFoundError:
            pass
        except OSError:
            ok = False
    return ok


@contextmanager
def _file_lock(path):
    os.makedirs(CONFIG_DIR, mode=_directory_mode(), exist_ok=True)
    _set_private_mode(CONFIG_DIR, 0o700)
    with open(path, "a+b") as lock:
        _set_private_mode(path, 0o600)
        if platform.system() == "Windows":
            import msvcrt
            if os.fstat(lock.fileno()).st_size == 0:
                lock.write(b"\0")
                lock.flush()
            lock.seek(0)
            msvcrt.locking(lock.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            if platform.system() == "Windows":
                lock.seek(0)
                msvcrt.locking(lock.fileno(), msvcrt.LK_UNLCK, 1)
            else:
                fcntl.flock(lock.fileno(), fcntl.LOCK_UN)


def _accounts_lock():
    return _file_lock(ACCOUNTS_LOCK_FILE)


def _session_lock():
    return _file_lock(SESSION_LOCK_FILE)
