"""Terminal raw keyboard input readers and animation spinner."""

import os
import platform
import select
import sys
import threading
import time

from agy_swap import ORANGE, RESET


class Spinner:
    def __init__(self, message="Loading...", delay=0.12):
        self.message = message
        self.delay = delay
        self.spinner_chars = ["·  ", "•  ", "●  ", "•  "]
        self.running = False
        self.thread = None
        self.enabled = sys.stdout.isatty()

    def spin(self):
        idx = 0
        while self.running:
            frame = self.spinner_chars[idx % len(self.spinner_chars)]
            sys.stdout.write(f"\r{ORANGE}{frame}{RESET} {self.message}")
            sys.stdout.flush()
            idx += 1
            time.sleep(self.delay)

    def __enter__(self):
        if not self.enabled:
            return self
        self.running = True
        self.thread = threading.Thread(target=self.spin, daemon=True)
        self.thread.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if not self.enabled:
            return
        self.running = False
        if self.thread:
            self.thread.join()
        sys.stdout.write("\r\033[K")
        sys.stdout.flush()


def _read_unix_key(fd):
    ch = os.read(fd, 1)
    if ch == b"\x1b":
        if not select.select([fd], [], [], 0.1)[0]:
            return "esc"
        ch2 = os.read(fd, 1)
        if ch2 not in (b"[", b"O") or not select.select([fd], [], [], 0.1)[0]:
            return ""
        ch3 = os.read(fd, 1)
        if ch3 == b"A":
            return "up"
        if ch3 == b"B":
            return "down"
        if ch3 == b"3" and select.select([fd], [], [], 0.1)[0] and os.read(fd, 1) == b"~":
            return "delete"
        return ""
    if ch in (b"\r", b"\n"):
        return "\n"
    if ch in (b"\x7f", b"\x08"):
        return "backspace"
    return ch.decode("utf-8", errors="ignore")


def _read_windows_key(getch):
    ch = getch()
    if ch in (b"\x00", b"\xe0"):
        return {b"H": "up", b"P": "down", b"S": "delete"}.get(getch(), "")
    if ch == b"\x1b":
        return "esc"
    if ch in (b"\r", b"\n"):
        return "\n"
    if ch in (b"\x08", b"\x7f"):
        return "backspace"
    return ch.decode("utf-8", errors="ignore")


def get_key():
    if platform.system() == "Windows":
        import msvcrt
        return _read_windows_key(msvcrt.getch)
    else:
        import tty, termios
        fd = sys.stdin.fileno()
        old_settings = termios.tcgetattr(fd)
        try:
            tty.setraw(fd)
            return _read_unix_key(fd)
        finally:
            termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)


def get_key_with_timeout(timeout=None):
    if timeout is None:
        return get_key()
    if platform.system() == "Windows":
        import msvcrt
        deadline = time.time() + timeout
        while time.time() < deadline:
            if msvcrt.kbhit():
                return _read_windows_key(msvcrt.getch)
            time.sleep(0.05)
        return None
    else:
        import tty, termios
        fd = sys.stdin.fileno()
        old_settings = termios.tcgetattr(fd)
        try:
            tty.setraw(fd)
            ready, _, _ = select.select([fd], [], [], timeout)
            if not ready:
                return None
            return _read_unix_key(fd)
        finally:
            termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)
