"""Network utilities and HTTP request helper with strict SSL verification by default."""

import os
import ssl
import sys
import urllib.error
import urllib.request


def allow_insecure_tls():
    """Check if insecure TLS verification bypass is explicitly enabled via environment variable."""
    val = os.environ.get("AGY_SWAP_INSECURE_TLS", "") or os.environ.get("AGY_SWAP_ALLOW_INSECURE_SSL", "")
    return val.strip().lower() in ("1", "true", "yes", "on")


def get_ssl_contexts():
    """Return SSL contexts to try. Strict default verification by default.
    Unverified fallback is only included if explicitly enabled via AGY_SWAP_INSECURE_TLS=1."""
    contexts = []
    try:
        contexts.append(ssl.create_default_context())
    except Exception:
        pass
    if allow_insecure_tls():
        try:
            contexts.append(ssl._create_unverified_context())
        except Exception:
            pass
    return contexts


def safe_urlopen(url_or_req, timeout=10):
    """Execute urlopen with strict SSL verification by default; insecure fallback requires opt-in."""
    if isinstance(url_or_req, str):
        req = urllib.request.Request(url_or_req, headers={"User-Agent": "Mozilla/5.0"})
    else:
        req = url_or_req
        if not req.has_header("User-agent") and not req.has_header("User-Agent"):
            req.add_header("User-Agent", "Mozilla/5.0")

    last_exc = None
    for ctx in get_ssl_contexts():
        try:
            if ctx.verify_mode == ssl.CERT_NONE:
                sys.stderr.write("Warning: AGY_SWAP_INSECURE_TLS is enabled; TLS certificate verification is disabled.\n")
                sys.stderr.flush()
            return urllib.request.urlopen(req, timeout=timeout, context=ctx)
        except urllib.error.HTTPError:
            # HTTP level errors (e.g. 404, 403, 401) mean TLS handshakes succeeded
            raise
        except (urllib.error.URLError, ssl.SSLError, OSError) as exc:
            last_exc = exc

    if last_exc:
        raise last_exc
    raise RuntimeError("safe_urlopen failed to establish connection")
