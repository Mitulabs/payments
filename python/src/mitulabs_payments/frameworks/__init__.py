"""Where each framework keeps its request, its response and the payment the view reads."""

import contextlib
import importlib
from collections.abc import Mapping
from typing import Any, Protocol

from ..credentials import PAYMENT_HEADERS
from ..models import Incoming, Payment, Reply

_FRAMEWORKS = ("fastapi", "django", "flask")


class HeaderLookup(Protocol):
    def get(self, key: str, /) -> str | None: ...


class Adapter(Protocol):
    def request_of(self, args: tuple[Any, ...], kwargs: Mapping[str, Any]) -> Any | None:
        """The framework request among a view's arguments, or None for another framework."""

    def incoming(self, request: Any) -> Incoming: ...

    def respond(self, reply: Reply) -> Any: ...

    def finish(self, result: Any, headers: Mapping[str, str]) -> Any:
        """The view's return value as a response carrying ``headers``."""

    def attach(self, request: Any, payment: Payment | None) -> None: ...


def installed_adapters() -> list[Adapter]:
    found: list[Adapter] = []
    for name in _FRAMEWORKS:
        with contextlib.suppress(ImportError):
            found.append(importlib.import_module(f"{__name__}.{name}").ADAPTER)
    return found


def payment_headers(headers: HeaderLookup) -> dict[str, str]:
    return {name: value for name in PAYMENT_HEADERS if (value := headers.get(name))}
