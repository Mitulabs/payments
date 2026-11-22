from dataclasses import dataclass
from decimal import Decimal


@dataclass(frozen=True, slots=True, kw_only=True)
class Payment:
    """A payment Mitu has settled for this request, as the view receives it."""

    payment_id: str
    reference: str
    tx_hash: str | None
    amount_paid: Decimal | None
    currency: str | None
    network: str | None
    payment_method: str | None


@dataclass(frozen=True, slots=True)
class Price:
    amount: Decimal
    currency: str
    description: str | None


@dataclass(frozen=True, slots=True)
class Incoming:
    """What the flow reads from a request: never its body, cookies or other headers."""

    method: str
    path: str
    headers: dict[str, str]


@dataclass(frozen=True, slots=True)
class Reply:
    """A response sent instead of running the view: a 402, Mitu's refusal, or a 503."""

    status: int
    headers: dict[str, str]
    body: bytes


@dataclass(frozen=True, slots=True)
class Paid:
    payment: Payment
    headers: dict[str, str]


Outcome = Reply | Paid
