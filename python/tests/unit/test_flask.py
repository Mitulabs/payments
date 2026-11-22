"""The whole flow, through Flask's test client."""

import json
import logging
import re
from decimal import Decimal
from typing import Any

import flask
import httpx
import pytest
from fake_mitu import (
    AGENT_HOST,
    AGENT_ORIGIN,
    API_KEY,
    CHALLENGE_BODY,
    PAYMENT_REQUIRED,
    PAYMENTS_PATH,
    RECEIPT,
    SOLANA_CHALLENGE,
    TX_HASH,
    X402_RECEIPT,
    FakeMitu,
)
from flask.testing import FlaskClient
from wire import MPP_AUTHORIZATION, MPP_CHALLENGE, PAYMENT_ID, X402_PAYMENT_SIGNATURE

from mitulabs_payments import Mitu, MituApiError, MituConfigError, Payment, __version__

MPP = {"Authorization": MPP_AUTHORIZATION}


@pytest.fixture
def views() -> list[Payment | None]:
    return []


@pytest.fixture
def client(mitu: Mitu, views: list[Payment | None]) -> FlaskClient:
    app = flask.Flask(__name__)
    app.testing = True

    @app.route("/report", methods=["GET", "POST"])
    @mitu.charge(amount="0.01", description="Market report")
    def report() -> dict[str, str]:
        views.append(flask.g.mitu_payment)
        return {"report": "ready"}

    @app.get("/naira")
    @mitu.charge(amount="1500", currency="ngn")
    def naira() -> str:
        views.append(flask.g.mitu_payment)
        return "paid"

    @app.get("/maybe")
    @mitu.charge(amount=lambda request: None if request.args.get("free") else "0.01")
    def maybe() -> str:
        views.append(flask.g.mitu_payment)
        return "served"

    return app.test_client()


def test_unpaid_request_creates_a_payment_and_passes_mitus_402_through(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    response = client.get("/report?q=lagos", headers={"Cookie": "session=s"})

    assert response.status_code == 402
    assert response.headers["WWW-Authenticate"] == f"{MPP_CHALLENGE}, {SOLANA_CHALLENGE}"
    assert response.headers["PAYMENT-REQUIRED"] == PAYMENT_REQUIRED
    assert response.headers["Cache-Control"] == "no-store"
    assert response.headers["Content-Type"].startswith("application/problem+json")
    assert "Set-Cookie" not in response.headers
    assert response.json == CHALLENGE_BODY
    assert views == []
    [created] = fake.created
    assert re.fullmatch(r"sdk_[0-9a-f]{32}", created.pop("reference"))
    assert created == {
        "amount": 0.01,
        "currency": "USD",
        "description": "Market report",
        "expires_at": "2030-01-01T12:15:00.000Z",
        "metadata": {"sdk": f"mitulabs-payments@{__version__}", "method": "GET", "path": "/report"},
    }
    [create] = fake.sent("POST", PAYMENTS_PATH)
    assert create.headers["x-api-key"] == API_KEY
    [challenge] = fake.agent_requests
    assert str(challenge.url) == f"{AGENT_ORIGIN}/pay/{fake.payments.popitem()[0]}"
    assert "x-api-key" not in challenge.headers
    assert "authorization" not in challenge.headers
    assert "cookie" not in challenge.headers


def test_each_unpaid_request_gets_its_own_reference(client: FlaskClient, fake: FakeMitu) -> None:
    client.get("/report")
    client.get("/report")

    first, second = (created["reference"] for created in fake.created)
    assert first != second


def test_paid_mpp_request_is_checked_forwarded_and_served_with_the_receipt(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    fake.add_payment()

    response = client.get(
        "/report?q=lagos", headers={**MPP, "Cookie": "session=s", "X-Merchant": "private"}
    )

    assert response.status_code == 200
    assert response.json == {"report": "ready"}
    assert response.headers["Payment-Receipt"] == RECEIPT
    assert response.headers["Cache-Control"] == "no-store"
    assert views == [
        Payment(
            payment_id=PAYMENT_ID,
            reference=f"sdk_ref_{PAYMENT_ID}",
            tx_hash=TX_HASH,
            amount_paid=Decimal("0.01"),
            currency="USDC",
            network="tempo",
            payment_method="mpp",
        )
    ]
    [lookup] = fake.sent("GET", PAYMENTS_PATH)
    assert lookup.url.path == f"{PAYMENTS_PATH}/{PAYMENT_ID}"
    assert lookup.headers["x-api-key"] == API_KEY
    [paying] = fake.agent_requests
    assert str(paying.url) == f"{AGENT_ORIGIN}/pay/{PAYMENT_ID}"
    assert paying.headers["authorization"] == MPP_AUTHORIZATION
    assert not {"x-api-key", "cookie", "x-merchant"} & set(paying.headers)
    assert fake.created == []


def test_mpp_credential_in_payment_authorization_is_forwarded_as_authorization(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    fake.add_payment()

    response = client.get(
        "/report",
        headers={
            "Authorization": "Bearer merchant-token",
            "Payment-Authorization": MPP_AUTHORIZATION,
        },
    )

    assert response.status_code == 200
    [paying] = fake.agent_requests
    assert paying.headers["authorization"] == MPP_AUTHORIZATION


@pytest.mark.parametrize("header", ["PAYMENT-SIGNATURE", "X-PAYMENT"])
def test_paid_x402_request_is_forwarded_and_served_with_the_receipt(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None], header: str
) -> None:
    fake.add_payment()

    response = client.get("/report", headers={header: X402_PAYMENT_SIGNATURE})

    assert response.status_code == 200
    assert response.headers["PAYMENT-RESPONSE"] == X402_RECEIPT
    assert views[0] and views[0].payment_id == PAYMENT_ID
    [paying] = fake.agent_requests
    assert paying.headers[header] == X402_PAYMENT_SIGNATURE
    assert "authorization" not in paying.headers


@pytest.mark.parametrize(
    "stored",
    [
        {"amount": "0.02"},
        {"amount": "0.001"},
        {"currency": "NGN"},
        {"path": "/cheap"},
        {"method": "POST"},
    ],
)
def test_payment_for_another_price_or_route_gets_a_fresh_402(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None], stored: dict[str, str]
) -> None:
    fake.add_payment(**stored)

    response = client.get("/report", headers=MPP)

    assert response.status_code == 402
    assert views == []
    assert len(fake.created) == 1
    assert all("authorization" not in request.headers for request in fake.agent_requests)


def test_payment_priced_in_a_local_currency_matches_regardless_of_case(
    client: FlaskClient, fake: FakeMitu
) -> None:
    fake.add_payment(amount="1500.00", currency="NGN", path="/naira")

    response = client.get("/naira", headers=MPP)

    assert response.status_code == 200
    assert response.text == "paid"


def test_payment_of_another_merchant_gets_a_fresh_402(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    response = client.get("/report", headers=MPP)

    assert response.status_code == 402
    assert views == []
    assert len(fake.created) == 1


@pytest.mark.parametrize(
    "headers",
    [
        {"Authorization": "Payment not-base64!"},
        {"Authorization": "Payment eyJub3QiOiJhIGNyZWRlbnRpYWwifQ"},
        {"Authorization": 'Payment id="abc", method="tempo"'},
        {"PAYMENT-SIGNATURE": "e30="},
        {"X-PAYMENT": "%%%"},
    ],
)
def test_unparseable_credential_gets_a_fresh_402(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None], headers: dict[str, str]
) -> None:
    response = client.get("/report", headers=headers)

    assert response.status_code == 402
    assert views == []
    assert fake.sent("GET", PAYMENTS_PATH) == []
    assert len(fake.created) == 1


@pytest.mark.parametrize(
    ("status", "slug"), [(403, "replay"), (409, "already-paid"), (410, "expired")]
)
def test_mitus_refusal_is_passed_through_without_running_the_view(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None], status: int, slug: str
) -> None:
    fake.add_payment()
    problem = {"type": f"https://paymentauth.org/problems/{slug}", "status": status}
    fake.agent_answer = (status, {"retry-after": "1", "x-internal": "1"}, problem)

    response = client.get("/report", headers=MPP)

    assert response.status_code == status
    assert response.json == problem
    assert response.headers["Retry-After"] == "1"
    assert response.headers["Cache-Control"] == "no-store"
    assert "X-Internal" not in response.headers
    assert views == []


def test_settlement_failure_with_a_fresh_challenge_is_passed_through(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    fake.add_payment()
    fake.agent_answer = (402, {"www-authenticate": MPP_CHALLENGE}, CHALLENGE_BODY)

    response = client.get("/report", headers=MPP)

    assert response.status_code == 402
    assert response.headers["WWW-Authenticate"] == MPP_CHALLENGE
    assert views == []


@pytest.mark.parametrize("error", [httpx.ConnectError, httpx.ReadTimeout])
@pytest.mark.parametrize("headers", [{}, MPP])
def test_unreachable_mitu_answers_503_with_retry_after(
    client: FlaskClient,
    fake: FakeMitu,
    views: list[Payment | None],
    error: type[httpx.TransportError],
    headers: dict[str, str],
) -> None:
    fake.add_payment()
    fake.down = error

    response = client.get("/report", headers=headers)

    assert response.status_code == 503
    assert response.headers["Retry-After"] == "5"
    assert response.headers["Cache-Control"] == "no-store"
    assert views == []


@pytest.mark.parametrize("status", [429, 500, 502, 503])
@pytest.mark.parametrize("headers", [{}, MPP])
def test_mitu_api_errors_answer_503(
    client: FlaskClient,
    fake: FakeMitu,
    views: list[Payment | None],
    status: int,
    headers: dict[str, str],
) -> None:
    fake.add_payment()
    fake.api_status = status

    response = client.get("/report", headers=headers)

    assert response.status_code == 503
    assert response.headers["Retry-After"] == "5"
    assert views == []


def test_failing_challenge_endpoint_answers_503(
    client: FlaskClient, fake: FakeMitu, monkeypatch: pytest.MonkeyPatch
) -> None:
    handle = fake.handle

    def broken_agent(request: httpx.Request) -> httpx.Response:
        return httpx.Response(502) if request.url.path.startswith("/pay/") else handle(request)

    monkeypatch.setattr(fake, "handle", broken_agent)

    assert client.get("/report").status_code == 503


def test_unreadable_payment_from_mitu_answers_503(
    client: FlaskClient, fake: FakeMitu, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(fake, "handle", lambda request: httpx.Response(201, json={"data": {}}))

    assert client.get("/report").status_code == 503


@pytest.mark.parametrize(
    "agent_url",
    [f"http://{AGENT_HOST}/pay/{PAYMENT_ID}", f"http://localhost/pay/{PAYMENT_ID}", "https://"],
)
def test_agent_url_off_https_is_never_called(
    client: FlaskClient, fake: FakeMitu, agent_url: str
) -> None:
    fake.add_payment()
    fake.payments[PAYMENT_ID]["agent_url"] = agent_url

    response = client.get("/report", headers=MPP)

    assert response.status_code == 503
    assert fake.agent_requests == []


def test_refused_api_key_raises_without_serving_the_route(
    fake: FakeMitu, views: list[Payment | None]
) -> None:
    mitu = Mitu("sk_test_" + "b2" * 24, transport=fake.transport)
    app = flask.Flask(__name__)
    app.testing = True

    @app.get("/report")
    @mitu.charge(amount="0.01")
    def report() -> str:
        views.append(flask.g.mitu_payment)
        return "served"

    with pytest.raises(MituApiError) as raised:
        app.test_client().get("/report")

    assert raised.value.status == 401
    assert str(raised.value) == f"Mitu API POST {PAYMENTS_PATH} failed (401: Unauthorized)"
    assert views == []


def test_free_request_runs_the_view_without_calling_mitu(
    client: FlaskClient, fake: FakeMitu, views: list[Payment | None]
) -> None:
    response = client.get("/maybe?free=1")

    assert response.status_code == 200
    assert response.text == "served"
    assert views == [None]
    assert fake.requests == []


def test_priced_request_of_a_dynamic_route_is_charged(client: FlaskClient, fake: FakeMitu) -> None:
    response = client.get("/maybe")

    assert response.status_code == 402
    assert fake.created[0]["metadata"]["path"] == "/maybe"


def test_api_key_and_credentials_are_never_logged_or_sent_to_agent_url(
    client: FlaskClient, fake: FakeMitu, caplog: pytest.LogCaptureFixture
) -> None:
    caplog.set_level(logging.DEBUG)
    fake.add_payment()

    client.get("/report")
    client.get("/report", headers=MPP)
    client.get("/report", headers={"PAYMENT-SIGNATURE": X402_PAYMENT_SIGNATURE})
    client.get("/naira", headers=MPP)
    fake.api_status = 503
    client.get("/report", headers=MPP)
    fake.down = httpx.ConnectError
    client.get("/report")

    secrets = (API_KEY, MPP_AUTHORIZATION.split()[1], X402_PAYMENT_SIGNATURE, RECEIPT)
    assert caplog.records
    assert not any(secret in caplog.text for secret in secrets)
    to_agent = [r for r in fake.requests if r.url.host == AGENT_HOST]
    assert to_agent
    assert all(
        API_KEY not in json.dumps(dict(request.headers)) and "x-api-key" not in request.headers
        for request in to_agent
    )


def test_async_price_on_a_sync_view_is_refused(mitu: Mitu) -> None:
    async def price(request: Any) -> str:
        return "0.01"

    app = flask.Flask(__name__)
    app.testing = True

    @app.get("/report")
    @mitu.charge(amount=price)
    def report() -> str:
        return "served"

    with pytest.raises(MituConfigError, match="async price"):
        app.test_client().get("/report")


def test_invalid_dynamic_price_is_refused(mitu: Mitu, fake: FakeMitu) -> None:
    app = flask.Flask(__name__)
    app.testing = True

    @app.get("/report")
    @mitu.charge(amount=lambda request: "0.005")
    def report() -> str:
        return "served"

    with pytest.raises(MituConfigError, match="2 decimal places"):
        app.test_client().get("/report")
    assert fake.requests == []
