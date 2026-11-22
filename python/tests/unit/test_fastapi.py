"""FastAPI, through its test client."""

from collections.abc import Iterator

import httpx
import pytest
from fake_mitu import (
    API_KEY,
    PAYMENT_REQUIRED,
    RECEIPT,
    TX_HASH,
    X402_RECEIPT,
    FakeMitu,
)
from fastapi import FastAPI, Request
from fastapi.responses import PlainTextResponse
from fastapi.testclient import TestClient
from wire import MPP_AUTHORIZATION, MPP_CHALLENGE, PAYMENT_ID, X402_PAYMENT_SIGNATURE

from mitulabs_payments import Mitu


@pytest.fixture
def client(mitu: Mitu) -> Iterator[TestClient]:
    app = FastAPI()

    @app.get("/report")
    @mitu.charge(amount="0.01", description="Market report")
    async def report(request: Request) -> dict[str, str | None]:
        return {"tx": request.state.mitu_payment.tx_hash}

    async def price_for(request: Request) -> str | None:
        return None if request.query_params.get("free") else "0.01"

    @app.get("/maybe")
    @mitu.charge(amount=price_for)
    async def maybe(request: Request) -> PlainTextResponse:
        return PlainTextResponse("paid" if request.state.mitu_payment else "free")

    @app.get("/sync")
    @mitu.charge(amount="0.01")
    def sync_report(request: Request) -> dict[str, str]:
        return {"payment": request.state.mitu_payment.payment_id}

    with TestClient(app) as test_client:
        yield test_client


def test_unpaid_request_gets_mitus_402(client: TestClient, fake: FakeMitu) -> None:
    response = client.get("/report")

    assert response.status_code == 402
    assert response.headers["www-authenticate"].startswith(MPP_CHALLENGE)
    assert response.headers["payment-required"] == PAYMENT_REQUIRED
    assert response.headers["cache-control"] == "no-store"
    assert fake.created[0]["metadata"]["path"] == "/report"


def test_paid_request_runs_the_route_and_adds_the_receipt(
    client: TestClient, fake: FakeMitu
) -> None:
    fake.add_payment()

    response = client.get("/report", headers={"Authorization": MPP_AUTHORIZATION})

    assert response.status_code == 200
    assert response.json() == {"tx": TX_HASH}
    assert response.headers["payment-receipt"] == RECEIPT
    assert response.headers["cache-control"] == "no-store"


def test_paid_x402_request_keeps_the_routes_own_response(
    client: TestClient, fake: FakeMitu
) -> None:
    fake.add_payment(path="/maybe")

    response = client.get("/maybe", headers={"PAYMENT-SIGNATURE": X402_PAYMENT_SIGNATURE})

    assert response.status_code == 200
    assert response.text == "paid"
    assert response.headers["payment-response"] == X402_RECEIPT


def test_free_request_skips_mitu(client: TestClient, fake: FakeMitu) -> None:
    response = client.get("/maybe?free=1")

    assert response.text == "free"
    assert fake.requests == []


def test_sync_route_is_charged_too(client: TestClient, fake: FakeMitu) -> None:
    fake.add_payment(path="/sync")

    assert client.get("/sync").status_code == 402
    paid = client.get("/sync", headers={"Authorization": MPP_AUTHORIZATION})
    assert paid.json() == {"payment": PAYMENT_ID}


def test_refusal_is_passed_through(client: TestClient, fake: FakeMitu) -> None:
    fake.add_payment()
    fake.agent_answer = (410, {}, {"type": "https://paymentauth.org/problems/expired"})

    response = client.get("/report", headers={"Authorization": MPP_AUTHORIZATION})

    assert response.status_code == 410
    assert response.json() == {"type": "https://paymentauth.org/problems/expired"}


@pytest.mark.anyio
async def test_close_releases_connections_and_later_requests_reconnect(fake: FakeMitu) -> None:
    mitu = Mitu(API_KEY, transport=fake.transport)
    app = FastAPI()

    @app.get("/report")
    @mitu.charge(amount="0.01")
    async def report(request: Request) -> str:
        return "paid"

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app), base_url="http://testserver"
    ) as client:
        assert (await client.get("/report")).status_code == 402
        await mitu.close()
        assert (await client.get("/report")).status_code == 402
    await mitu.close()
