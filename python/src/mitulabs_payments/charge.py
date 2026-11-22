"""One request to a charged route, written once as a generator of HTTP requests and driven by a
sync or an async client."""

import json
import logging
import secrets
from collections.abc import Generator, Iterable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from decimal import Decimal, InvalidOperation
from typing import Final, cast
from urllib.parse import urlsplit

import httpx

from ._version import __version__
from .api import MituApi
from .credentials import Credential, credential_of
from .errors import MituApiError
from .models import Incoming, Outcome, Paid, Payment, Price, Reply

PAYMENT_TTL: Final = timedelta(minutes=15)
RETRY_AFTER_SECONDS: Final = 5

Flow = Generator[httpx.Request, httpx.Response, Outcome]

_SDK: Final = f"mitulabs-payments@{__version__}"
_RELAYED_HEADERS: Final = ("Content-Type", "WWW-Authenticate", "PAYMENT-REQUIRED", "Retry-After")
_RECEIPT_HEADERS: Final = ("Payment-Receipt", "PAYMENT-RESPONSE")
_NO_STORE: Final = {"Cache-Control": "no-store"}
_MAX_ERROR_CODE_LENGTH: Final = 64

UNAVAILABLE: Final = Reply(
    503,
    {
        "Content-Type": "application/problem+json",
        "Retry-After": str(RETRY_AFTER_SECONDS),
        **_NO_STORE,
    },
    json.dumps(
        {"type": "about:blank", "title": "Payment service unavailable", "status": 503}
    ).encode(),
)

logger = logging.getLogger(__name__)


class _Unavailable(Exception):
    pass


@dataclass(frozen=True, slots=True)
class _Stored:
    payment_id: str
    reference: str
    amount: Decimal
    currency: str
    agent_url: str
    metadata: Mapping[str, object]


def charge(api: MituApi, price: Price, incoming: Incoming) -> Flow:
    if (credential := credential_of(incoming.headers)) and (
        stored := (yield from _bound_payment(api, credential, price, incoming))
    ):
        response = yield api.agent(stored.agent_url, credential.forward)
        if response.status_code == 200:
            receipt = {**_copy(response, _RECEIPT_HEADERS), **_NO_STORE}
            return Paid(_payment(stored, response), receipt)
        return _relay(response)
    return (yield from _challenge(api, price, incoming))


def run(api: MituApi, flow: Flow) -> Outcome:
    try:
        request = next(flow)
        while True:
            request = flow.send(api.send(request))
    except StopIteration as done:
        return cast(Outcome, done.value)
    except (httpx.RequestError, _Unavailable) as error:
        return _unavailable(error)
    finally:
        flow.close()


async def arun(api: MituApi, flow: Flow) -> Outcome:
    try:
        request = next(flow)
        while True:
            request = flow.send(await api.asend(request))
    except StopIteration as done:
        return cast(Outcome, done.value)
    except (httpx.RequestError, _Unavailable) as error:
        return _unavailable(error)
    finally:
        flow.close()


def now() -> datetime:
    return datetime.now(UTC)


def _bound_payment(
    api: MituApi, credential: Credential, price: Price, incoming: Incoming
) -> Generator[httpx.Request, httpx.Response, _Stored | None]:
    """The credential's payment, if it is this merchant's and priced for this very route."""
    response = yield api.get_payment(credential.payment_id)
    if response.status_code in (400, 404):
        logger.info("Payment %s is not this merchant's; issuing a new one", credential.payment_id)
        return None
    stored = _stored(_data(response))
    if (
        stored.payment_id == credential.payment_id
        and stored.amount == price.amount
        and stored.currency == price.currency
        and stored.metadata.get("method") == incoming.method
        and stored.metadata.get("path") == incoming.path
    ):
        return stored
    logger.info("Payment %s is for another route or price; issuing a new one", stored.payment_id)
    return None


def _challenge(api: MituApi, price: Price, incoming: Incoming) -> Flow:
    created = yield api.create_payment(_new_payment(price, incoming))
    stored = _stored(_data(created))
    response = yield api.agent(stored.agent_url, {})
    if response.is_server_error:
        raise _Unavailable(f"agent_url answered {response.status_code}")
    return _relay(response)


def _new_payment(price: Price, incoming: Incoming) -> dict[str, object]:
    expires_at = now() + PAYMENT_TTL
    return {
        # Exact: parse_price only accepts amounts that survive the float.
        "amount": float(price.amount),
        "currency": price.currency,
        "reference": f"sdk_{secrets.token_hex(16)}",
        **({"description": price.description} if price.description else {}),
        "expires_at": expires_at.isoformat(timespec="milliseconds").replace("+00:00", "Z"),
        "metadata": {"sdk": _SDK, "method": incoming.method, "path": incoming.path},
    }


def _data(response: httpx.Response) -> Mapping[str, object]:
    status = response.status_code
    if status == 429 or response.is_server_error:
        raise _Unavailable(f"Mitu API answered {status}")
    if not response.is_success:
        call = f"{response.request.method} {response.request.url.path}"
        raise MituApiError(status, f"Mitu API {call} failed ({status}{_error_code(response)})")
    match _json(response):
        case {"data": dict(data)}:
            return data
    raise _Unavailable("Mitu API answered without a payment")


def _stored(data: Mapping[str, object]) -> _Stored:
    match data:
        case {
            "id": str(payment_id),
            "reference": str(reference),
            "amount": str(amount),
            "currency": str(currency),
            "agent_url": str(agent_url),
        } if (value := _decimal(amount)) is not None and _is_https(agent_url):
            metadata = data.get("metadata")
            return _Stored(
                payment_id,
                reference,
                value,
                currency.upper(),
                agent_url,
                metadata if isinstance(metadata, dict) else {},
            )
    raise _Unavailable("Mitu API answered with an unreadable payment")


def _is_https(url: str) -> bool:
    try:
        parts = urlsplit(url)
        _ = parts.port
    except ValueError:
        return False
    return parts.scheme == "https" and bool(parts.hostname)


def _payment(stored: _Stored, response: httpx.Response) -> Payment:
    match _json(response):
        case {"receipt": dict(receipt)}:
            pass
        case _:
            receipt = {}
    return Payment(
        payment_id=stored.payment_id,
        reference=stored.reference,
        tx_hash=_text(receipt.get("tx_hash")),
        amount_paid=_decimal(receipt.get("amount_paid")),
        currency=_text(receipt.get("currency")),
        network=_text(receipt.get("network")),
        payment_method=_text(receipt.get("payment_method")),
    )


def _relay(response: httpx.Response) -> Reply:
    return Reply(
        response.status_code,
        {**_copy(response, _RELAYED_HEADERS), **_NO_STORE},
        response.content,
    )


def _copy(response: httpx.Response, names: Iterable[str]) -> dict[str, str]:
    return {
        name: ", ".join(values) for name in names if (values := response.headers.get_list(name))
    }


def _unavailable(error: Exception) -> Reply:
    reason = str(error) if isinstance(error, _Unavailable) else type(error).__name__
    logger.warning("Mitu is unavailable (%s); answering 503", reason)
    return UNAVAILABLE


def _json(response: httpx.Response) -> object:
    try:
        return response.json()
    except ValueError:
        return None


def _error_code(response: httpx.Response) -> str:
    match _json(response):
        case {"error": str(code)}:
            return f": {code[:_MAX_ERROR_CODE_LENGTH]}"
    return ""


def _decimal(value: object) -> Decimal | None:
    if isinstance(value, bool) or not isinstance(value, str | int | float):
        return None
    try:
        amount = Decimal(str(value))
    except InvalidOperation:
        return None
    return amount if amount.is_finite() else None


def _text(value: object) -> str | None:
    return value if isinstance(value, str) and value else None
