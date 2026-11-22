"""Django, through its test client, with sync and async views."""

from collections.abc import Iterator
from typing import Any

import django
import pytest
from django.conf import settings
from django.http import HttpRequest, HttpResponse, JsonResponse
from django.test import Client
from django.urls import path
from fake_mitu import PAYMENT_REQUIRED, RECEIPT, TX_HASH, FakeMitu
from wire import MPP_AUTHORIZATION, X402_PAYMENT_SIGNATURE

from mitulabs_payments import Mitu, __version__

urlpatterns: list[Any] = []

if not settings.configured:
    settings.configure(ROOT_URLCONF=__name__, ALLOWED_HOSTS=["testserver"], USE_TZ=True)
    django.setup()


@pytest.fixture
def client(mitu: Mitu) -> Iterator[Client]:
    @mitu.charge(amount="0.01", description="Market report")
    def report(request: HttpRequest) -> HttpResponse:
        return JsonResponse({"tx": request.mitu_payment.tx_hash})

    @mitu.charge(amount=lambda request: None if request.GET.get("free") else "0.01")
    def maybe(request: HttpRequest) -> HttpResponse:
        return HttpResponse("paid" if request.mitu_payment else "free")

    @mitu.charge(amount="0.01")
    async def report_async(request: HttpRequest) -> HttpResponse:
        return JsonResponse({"tx": request.mitu_payment.tx_hash})

    urlpatterns[:] = [path("report", report), path("maybe", maybe), path("async", report_async)]
    yield Client()
    urlpatterns.clear()


@pytest.mark.parametrize("route", ["/report", "/async"])
def test_unpaid_request_gets_mitus_402(client: Client, fake: FakeMitu, route: str) -> None:
    response = client.get(route)

    assert response.status_code == 402
    assert response.headers["PAYMENT-REQUIRED"] == PAYMENT_REQUIRED
    assert response.headers["Cache-Control"] == "no-store"
    assert fake.created[0]["metadata"] == {
        "sdk": f"mitulabs-payments@{__version__}",
        "method": "GET",
        "path": route,
    }


@pytest.mark.parametrize("route", ["/report", "/async"])
def test_paid_request_runs_the_view_and_adds_the_receipt(
    client: Client, fake: FakeMitu, route: str
) -> None:
    fake.add_payment(path=route)

    response = client.get(route, headers={"Authorization": MPP_AUTHORIZATION})

    assert response.status_code == 200
    assert response.json() == {"tx": TX_HASH}
    assert response.headers["Payment-Receipt"] == RECEIPT


def test_payment_for_another_view_gets_a_fresh_402(client: Client, fake: FakeMitu) -> None:
    fake.add_payment(path="/async")

    response = client.get("/report", headers={"PAYMENT-SIGNATURE": X402_PAYMENT_SIGNATURE})

    assert response.status_code == 402
    assert len(fake.created) == 1


def test_free_request_skips_mitu(client: Client, fake: FakeMitu) -> None:
    response = client.get("/maybe?free=1")

    assert response.content == b"free"
    assert fake.requests == []


def test_mitu_down_answers_503(client: Client, fake: FakeMitu) -> None:
    fake.api_status = 500

    response = client.get("/async")

    assert response.status_code == 503
    assert response.headers["Retry-After"] == "5"
