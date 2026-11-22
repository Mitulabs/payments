import re
from dataclasses import dataclass
from typing import Final, Literal
from urllib.parse import urlsplit

from .errors import MituConfigError

Environment = Literal["sandbox", "production"]

BASE_URL: Final = "https://api.mitu.ai/economy"

_API_KEY = re.compile(r"sk_(test|live)_[A-Za-z0-9]{16,}")
_LOCAL_HOSTS = frozenset({"localhost", "127.0.0.1", "::1"})


@dataclass(frozen=True, slots=True)
class ResolvedEnvironment:
    environment: Environment
    base_url: str


def resolve_environment(api_key: str, base_url: str | None = None) -> ResolvedEnvironment:
    """Derives the environment from the key: ``sk_test_`` is sandbox, ``sk_live_`` production."""
    if not (isinstance(api_key, str) and (key := _API_KEY.fullmatch(api_key))):
        raise MituConfigError("api_key must be a Mitu secret key (sk_test_... or sk_live_...)")
    environment: Environment = "production" if key[1] == "live" else "sandbox"
    return ResolvedEnvironment(environment, BASE_URL if base_url is None else _base(base_url))


def is_secure_url(url: str) -> bool:
    """True for https URLs, and for http ones that point at this machine."""
    try:
        parts = urlsplit(url)
        host = parts.hostname
        _ = parts.port
    except ValueError:
        return False
    return bool(host) and (
        parts.scheme == "https" or (parts.scheme == "http" and host in _LOCAL_HOSTS)
    )


def _base(url: str) -> str:
    if not is_secure_url(url):
        raise MituConfigError("base_url must be an https URL")
    parts = urlsplit(url)
    host = parts.hostname or ""
    netloc = f"[{host}]" if ":" in host else host
    port_suffix = "" if parts.port is None else f":{parts.port}"
    return f"{parts.scheme}://{netloc}{port_suffix}{parts.path.rstrip('/')}"
