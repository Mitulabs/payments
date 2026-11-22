"""Reading the Mitu payment id from an MPP or x402 credential.

The id only selects the payment to look up and settle. Mitu verifies the credential itself, so
nothing here trusts it.
"""

import base64
import json
import re
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final
from urllib.parse import urlsplit

MPP_HEADERS: Final = ("authorization", "payment-authorization")
X402_HEADERS: Final = ("payment-signature", "x-payment")
PAYMENT_HEADERS: Final = (*MPP_HEADERS, *X402_HEADERS)

PAYMENT_ID = re.compile(r"pmt_[0-9a-z]{1,46}")

_MAX_ENCODED_LENGTH = 32_768
_TO_STANDARD_BASE64 = str.maketrans("-_", "+/")


@dataclass(frozen=True, slots=True)
class Credential:
    payment_id: str
    forward: dict[str, str]


def credential_of(headers: Mapping[str, str]) -> Credential | None:
    """The first credential in ``headers`` naming a Mitu payment, with the headers to forward.

    An MPP credential is always forwarded as ``Authorization``, the only header Mitu reads it from.
    """
    for name in MPP_HEADERS:
        if (token := _payment_token(headers.get(name))) and (payment_id := _mpp_scope(token)):
            return Credential(payment_id, {"authorization": f"Payment {token}"})
    for name in X402_HEADERS:
        if (value := headers.get(name)) and (payment_id := _x402_resource_id(value)):
            return Credential(payment_id, {name: value})
    return None


def _payment_token(value: str | None) -> str | None:
    match (value or "").split(maxsplit=1):
        case [scheme, token] if scheme.lower() == "payment":
            return token.strip()
    return None


def _mpp_scope(token: str) -> str | None:
    match _decode(token):
        case {"challenge": {"opaque": str(opaque)}}:
            meta = _decode(opaque)
        case {"challenge": {"opaque": dict() as meta}}:
            pass
        case _:
            return None
    match meta:
        case {"_mppx_scope": str(scope)} if PAYMENT_ID.fullmatch(scope):
            return scope
    return None


def _x402_resource_id(value: str) -> str | None:
    match _decode(value):
        case {"resource": {"url": str(url)}}:
            try:
                path = urlsplit(url).path
            except ValueError:
                return None
            match path.rsplit("/", 2):
                case [_, "pay", payment_id] if PAYMENT_ID.fullmatch(payment_id):
                    return payment_id
    return None


def _decode(text: str) -> object:
    """Base64 (standard or URL-safe, padded or not) JSON, or None."""
    if len(text) > _MAX_ENCODED_LENGTH:
        return None
    padded = text.translate(_TO_STANDARD_BASE64) + "=" * (-len(text) % 4)
    try:
        return json.loads(base64.b64decode(padded, validate=True))
    except (ValueError, RecursionError):
        return None
