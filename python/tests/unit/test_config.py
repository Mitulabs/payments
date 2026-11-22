import pytest
from fake_mitu import API_KEY

from mitulabs_payments import Mitu, MituConfigError
from mitulabs_payments.environment import resolve_environment


@pytest.mark.parametrize(
    ("key", "base_url", "environment"),
    [
        (API_KEY, None, "sandbox"),
        ("sk_live_" + "c3" * 24, None, "production"),
        (API_KEY, "http://localhost:3000", "sandbox"),
        (API_KEY, "https://staging.example.com/", "sandbox"),
    ],
)
def test_environment_follows_the_key(key: str, base_url: str | None, environment: str) -> None:
    mitu = Mitu(key, base_url=base_url)

    assert mitu.environment == environment
    assert key not in repr(mitu)
    assert repr(mitu) == f"Mitu(environment={environment!r})"


@pytest.mark.parametrize("key", ["", "pk_test_" + "a" * 48, "sk_test_short", "sk_prod_" + "a" * 48])
def test_refuses_anything_but_a_secret_key(key: str) -> None:
    with pytest.raises(MituConfigError, match="secret key"):
        Mitu(key)


@pytest.mark.parametrize("base_url", ["http://api.mitu.ai", "ftp://localhost", "", "https://"])
def test_refuses_an_insecure_base_url(base_url: str) -> None:
    with pytest.raises(MituConfigError, match="https"):
        Mitu(API_KEY, base_url=base_url)


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


@pytest.mark.parametrize(
    ("base_url", "expected"),
    [
        (None, "https://api.mitu.ai/economy"),
        ("https://staging.example.com/economy/", "https://staging.example.com/economy"),
        ("http://localhost:4003", "http://localhost:4003"),
    ],
)
def test_calls_the_mitu_api(base_url: str | None, expected: str) -> None:
    for key in (API_KEY, "sk_live_" + "c3" * 24):
        assert resolve_environment(key, base_url).base_url == expected
