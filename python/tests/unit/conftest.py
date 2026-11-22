from collections.abc import Iterator
from datetime import UTC, datetime

import pytest
from fake_mitu import API_KEY, FakeMitu

import mitulabs_payments.charge as flow
from mitulabs_payments import Mitu

NOW = datetime(2030, 1, 1, 12, 0, tzinfo=UTC)


@pytest.fixture(autouse=True)
def fixed_clock(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(flow, "now", lambda: NOW)


@pytest.fixture
def fake() -> FakeMitu:
    return FakeMitu()


@pytest.fixture
def mitu(fake: FakeMitu) -> Iterator[Mitu]:
    instance = Mitu(API_KEY, transport=fake.transport)
    yield instance
    instance.close_sync()
