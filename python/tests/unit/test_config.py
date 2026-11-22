import pytest
from fake_mitu import API_KEY
from wire import PAYMENT_ID

from mitulabs_payments import Mitu, MituConfigError
from mitulabs_payments.environment import resolve_environment

LIVE_KEY = "sk_live_" + "c3" * 24


@pytest.mark.parametrize(("key", "environment"), [(API_KEY, "sandbox"), (LIVE_KEY, "production")])
def test_environment_follows_the_key(key: str, environment: str) -> None:
    mitu = Mitu(key)

    assert mitu.environment == environment
    assert key not in repr(mitu)
    assert repr(mitu) == f"Mitu(environment={environment!r})"


@pytest.mark.parametrize("key", ["", "pk_test_" + "a" * 48, "sk_test_short", "sk_prod_" + "a" * 48])
def test_refuses_anything_but_a_secret_key(key: str) -> None:
    with pytest.raises(MituConfigError, match="secret key"):
        Mitu(key)


@pytest.mark.parametrize(
    ("key", "api_url"),
    [(LIVE_KEY, "https://api.mitu.ai"), (API_KEY, "https://sandbox.mitu.ai")],
)
def test_calls_the_api_of_the_keys_environment(key: str, api_url: str) -> None:
    assert resolve_environment(key).api_url == api_url


@pytest.mark.parametrize(
    ("key", "payments_url"),
    [
        (LIVE_KEY, "https://api.mitu.ai/v1/payments"),
        (API_KEY, "https://sandbox.mitu.ai/v1/payments"),
    ],
)
def test_sends_payments_to_the_api_of_the_keys_environment(key: str, payments_url: str) -> None:
    api = Mitu(key)._api

    assert str(api.create_payment({}).url) == payments_url
    assert str(api.get_payment(PAYMENT_ID).url) == f"{payments_url}/{PAYMENT_ID}"


@pytest.mark.parametrize("currency", ["", "U", "US$", "DOLLARSXXXX", 840])
def test_refuses_an_invalid_currency(currency: str) -> None:
    with pytest.raises(MituConfigError, match="currency"):
        Mitu(API_KEY, default_currency=currency)


def test_uppercases_currencies() -> None:
    assert Mitu(API_KEY, default_currency="ngn").default_currency == "NGN"


@pytest.mark.parametrize("amount", ["0", "0.00", "0.001", "-1", "1e3", " 1", "1.", "abc", ""])
def test_refuses_an_invalid_price(amount: str) -> None:
    with pytest.raises(MituConfigError, match="amount"):
        Mitu(API_KEY).charge(amount)


def test_refuses_a_price_too_precise_for_a_json_number() -> None:
    with pytest.raises(MituConfigError, match="too precise"):
        Mitu(API_KEY).charge("12345678901234567.25")


@pytest.mark.parametrize("amount", [1, 0.5, None])
def test_refuses_a_price_that_is_not_a_string_or_function(amount: object) -> None:
    with pytest.raises(MituConfigError, match="amount"):
        Mitu(API_KEY).charge(amount)  # type: ignore[arg-type]


def test_refuses_an_overlong_description() -> None:
    with pytest.raises(MituConfigError, match="description"):
        Mitu(API_KEY).charge("1", description="x" * 501)


def test_refuses_a_view_without_a_request() -> None:
    @Mitu(API_KEY).charge("1")
    def view() -> str:
        return "served"

    with pytest.raises(MituConfigError, match="no request"):
        view()
