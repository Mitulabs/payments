import re
from decimal import Decimal

from .errors import MituConfigError

_AMOUNT = re.compile(r"[0-9]+(?:\.[0-9]{1,2})?")
_CURRENCY = re.compile(r"[A-Za-z]{2,10}")
MAX_DESCRIPTION_LENGTH = 500


def parse_price(value: object) -> Decimal:
    """Parses a price such as ``"5000"`` or ``"12.50"``: positive, at most two decimal places."""
    if not (isinstance(value, str) and _AMOUNT.fullmatch(value)):
        raise MituConfigError(
            f"amount must be a decimal string with at most 2 decimal places, got {value!r}"
        )
    amount = Decimal(value)
    if not amount:
        raise MituConfigError("amount must be greater than 0; return None to serve free")
    # Mitu reads the amount as a JSON number, so it must survive the trip through a float.
    if Decimal(repr(float(amount))) != amount:
        raise MituConfigError(f"amount {value} is too precise to send to Mitu")
    return amount


def parse_currency(value: object) -> str:
    if not (isinstance(value, str) and _CURRENCY.fullmatch(value)):
        raise MituConfigError(f"currency must be a currency code such as USD, got {value!r}")
    return value.upper()


def parse_description(value: object) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str) or len(value) > MAX_DESCRIPTION_LENGTH:
        raise MituConfigError(
            f"description must be a string of at most {MAX_DESCRIPTION_LENGTH} characters"
        )
    return value or None
