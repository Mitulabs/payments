import re
from dataclasses import dataclass
from typing import Final, Literal

from .errors import MituConfigError

Environment = Literal["sandbox", "production"]
KeyMode = Literal["test", "live"]

API_URLS: Final[dict[KeyMode, str]] = {
    "live": "https://api.mitu.ai",
    "test": "https://sandbox.mitu.ai",
}

_API_KEY = re.compile(r"sk_(test|live)_[A-Za-z0-9]{16,}")


@dataclass(frozen=True, slots=True)
class ResolvedEnvironment:
    environment: Environment
    api_url: str


def resolve_environment(api_key: str) -> ResolvedEnvironment:
    """Derives the environment and its API from the key: ``sk_test_`` is the sandbox,
    ``sk_live_`` production."""
    if not (isinstance(api_key, str) and (key := _API_KEY.fullmatch(api_key))):
        raise MituConfigError("api_key must be a Mitu secret key (sk_test_... or sk_live_...)")
    mode: KeyMode = "live" if key[1] == "live" else "test"
    environment: Environment = "production" if mode == "live" else "sandbox"
    return ResolvedEnvironment(environment, API_URLS[mode])
