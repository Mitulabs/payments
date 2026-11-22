import base64
import json

import pytest
from wire import MPP_AUTHORIZATION, PAYMENT_ID, X402_PAYMENT_SIGNATURE

from mitulabs_payments.credentials import Credential, credential_of


def b64url(value: object) -> str:
    return base64.urlsafe_b64encode(json.dumps(value).encode()).decode().rstrip("=")


def b64(value: object) -> str:
    return base64.b64encode(json.dumps(value).encode()).decode()


def mpp(opaque: object) -> str:
    return f"Payment {b64url({'challenge': {'id': 'c', 'opaque': opaque}, 'payload': {}})}"


def test_reads_the_scope_of_a_pympp_credential() -> None:
    assert credential_of({"authorization": MPP_AUTHORIZATION}) == Credential(
        PAYMENT_ID, {"authorization": MPP_AUTHORIZATION}
    )


def test_reads_payment_authorization_and_forwards_it_as_authorization() -> None:
    credential = credential_of(
        {"authorization": "Bearer merchant", "payment-authorization": MPP_AUTHORIZATION}
    )

    assert credential == Credential(PAYMENT_ID, {"authorization": MPP_AUTHORIZATION})


def test_reads_the_legacy_object_opaque_of_older_mppx_clients() -> None:
    header = mpp({"_mppx_scope": PAYMENT_ID})

    assert credential_of({"authorization": header}) == Credential(
        PAYMENT_ID, {"authorization": header}
    )


def test_accepts_any_case_and_spacing_of_the_scheme() -> None:
    token = MPP_AUTHORIZATION.split()[1]

    credential = credential_of({"authorization": f"  payment   {token} "})

    assert credential == Credential(PAYMENT_ID, {"authorization": f"Payment {token}"})


@pytest.mark.parametrize("name", ["payment-signature", "x-payment"])
def test_reads_the_resource_of_an_x402_payment(name: str) -> None:
    assert credential_of({name: X402_PAYMENT_SIGNATURE}) == Credential(
        PAYMENT_ID, {name: X402_PAYMENT_SIGNATURE}
    )


def test_prefers_mpp_when_both_are_sent() -> None:
    credential = credential_of(
        {"authorization": MPP_AUTHORIZATION, "payment-signature": X402_PAYMENT_SIGNATURE}
    )

    assert credential and credential.forward == {"authorization": MPP_AUTHORIZATION}


@pytest.mark.parametrize(
    "headers",
    [
        {},
        {"authorization": "Bearer token"},
        {"authorization": "Payment"},
        {"authorization": "Payment !!!"},
        {"authorization": 'Payment id="x", scheme="eip3009"'},
        {"authorization": mpp(b64url({"other": PAYMENT_ID}))},
        {"authorization": mpp(b64url({"_mppx_scope": "pl_0123"}))},
        {"authorization": mpp(b64url({"_mppx_scope": "pmt_../../v1/me"}))},
        {"authorization": mpp(b64url({"_mppx_scope": 7}))},
        {"authorization": mpp(["_mppx_scope"])},
        {"authorization": f"Payment {b64url([1, 2])}"},
        {"authorization": f"Payment {'A' * 40_000}"},
        {"authorization": f"Payment {b64url('[' * 5000)}"},
        {"payment-signature": b64({"x402Version": 1, "scheme": "exact", "payload": {}})},
        {"payment-signature": b64({"resource": {"url": f"https://api.mitu.ai/x/{PAYMENT_ID}"}})},
        {"payment-signature": b64({"resource": {"url": "https://api.mitu.ai/pay/pmt_A"}})},
        {"payment-signature": b64({"resource": {"url": "http://[::1"}})},
        {"payment-signature": b64({"resource": f"https://api.mitu.ai/pay/{PAYMENT_ID}"})},
        {"x-payment": "not base64"},
    ],
)
def test_finds_no_payment_in_anything_else(headers: dict[str, str]) -> None:
    assert credential_of(headers) is None
