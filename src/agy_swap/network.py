"""Network utilities and HTTP request helper with SSL context fallback."""

import ssl
import urllib.error
import urllib.request


def get_ssl_contexts():
    """Return SSL contexts to try in order (default verified first, then unverified fallback)."""
    contexts = []
    try:
        contexts.append(ssl.create_default_context())
    except Exception:
        pass
    try:
        ctx = ssl._create_unverified_context()
        contexts.append(ctx)
    except Exception:
        pass
    return contexts


def safe_urlopen(url_or_req, timeout=10):
    """Execute urlopen with SSL context fallback for environments with self-signed certs or missing CA roots."""
    if isinstance(url_or_req, str):
        req = urllib.request.Request(url_or_req, headers={"User-Agent": "Mozilla/5.0"})
    else:
        req = url_or_req
        if not req.has_header("User-agent") and not req.has_header("User-Agent"):
            req.add_header("User-Agent", "Mozilla/5.0")

    last_exc = None
    for ctx in get_ssl_contexts():
        try:
            return urllib.request.urlopen(req, timeout=timeout, context=ctx)
        except urllib.error.HTTPError:
            # HTTP level errors (e.g. 404, 403, 401) mean TLS handshakes succeeded
            raise
        except (urllib.error.URLError, ssl.SSLError, OSError) as exc:
            last_exc = exc

    if last_exc:
        raise last_exc
    raise RuntimeError("safe_urlopen failed to establish connection")
