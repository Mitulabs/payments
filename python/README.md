# mitulabs-payments

Python SDK for charging AI agents per request on HTTP APIs. It places Mitu's payment flow in front of a route:
agents pay through the [Machine Payments Protocol](https://mpp.dev) (MPP) or [x402](https://x402.org), and Mitu
issues the payment challenges, settles each payment on chain, and delivers the `payment.completed` webhook.

The SDK does not verify payments, hold receiving addresses, or convert currencies. For each request it creates a
payment through the Mitu API, confirms that the payment an agent presents belongs to the route and price being
requested, and serves the route once Mitu has settled it. Prices in local currencies are converted at the rate
Mitu locks when the payment is created.

## Requirements

- Python 3.12 or later
- FastAPI, Flask or Django

## Installation

```bash
pip install "mitulabs-payments[fastapi]"   # or [flask], [django]
```

## Configuration

```python
import os

from mitulabs_payments import Mitu

mitu = Mitu(os.environ["MITU_API_KEY"])
```

| Option | Default | Description |
| --- | --- | --- |
| `api_key` | required | Mitu secret key. `sk_test_` selects the sandbox, `sk_live_` production. |
| `default_currency` | `"USD"` | Currency code for prices that do not specify one. |
| `base_url` | Mitu API for the key's environment | API origin override, for testing. Must use HTTPS unless it is `localhost`. |
| `transport` | — | An `httpx` transport, for testing. |

The API key is sent only to the Mitu API, as `x-api-key`. It is never forwarded to an agent payment URL, logged,
or included in an error message.

## Usage

Decorate a route with `@mitu.charge`. The decorator sits below the framework's route decorator.

### FastAPI

```python
from fastapi import FastAPI, Request

app = FastAPI()


@app.get("/v1/search")
@mitu.charge(amount="0.01", currency="USD", description="Search")
async def search(request: Request):
    return run_search(request.query_params, request.state.mitu_payment)
```

Charged FastAPI routes accept `request: Request`. Both `async def` and `def` routes are supported.

### Flask

```python
from flask import g, request


@app.get("/v1/search")
@mitu.charge(amount="0.01", currency="USD")
def search():
    return run_search(request.args, g.mitu_payment)
```

### Django

```python
@mitu.charge(amount="0.01", currency="USD")
def search(request):
    return JsonResponse(run_search(request.GET, request.mitu_payment))
```

Sync and async Django views are supported.

### Shutdown

Call `await mitu.close()` (or `mitu.close_sync()` in sync applications) at shutdown to release HTTP connections.

## Request flow

1. **No payment credential.** The SDK creates a payment for the route's price, with a 15-minute expiry and the
   route's method and path in its metadata, and returns Mitu's `402 Payment Required` response, including the MPP
   `WWW-Authenticate` and x402 `PAYMENT-REQUIRED` challenges.
2. **Payment credential present.** The SDK reads the Mitu payment id from the MPP or x402 credential, retrieves the
   payment, and checks that its amount, currency, method and path match the route. A credential for any other
   route or price receives a new `402`.
3. **Settlement.** The SDK forwards only the payment headers to the payment's `agent_url`. When Mitu confirms
   settlement, the view runs and the response carries Mitu's receipt header (`Payment-Receipt` or
   `PAYMENT-RESPONSE`). Any other answer from Mitu, such as a replayed or expired payment, is returned to the agent
   unchanged, and the view does not run.

Every response that carries payment state includes `Cache-Control: no-store`. If Mitu cannot be reached, or
answers with a server error, the route responds `503 Service Unavailable` with `Retry-After: 5` and is not served.

## Pricing

`amount` is a decimal string greater than zero with at most two decimal places, such as `"0.01"` or `"1500"`. It
can also be a function of the request that returns such a string, or `None` to serve that request without payment.
On async views the function may be async. The function is evaluated on the initial request and on the paid retry,
and must return the same price for both. Derive prices from server-side state, never from the request body.

## Payments

The view receives a `Payment` with:

| Field | Description |
| --- | --- |
| `payment_id` | The Mitu payment id (`pmt_...`). |
| `reference` | The payment reference the SDK generated. |
| `tx_hash` | The settlement transaction hash, where the payment method has one. |
| `amount_paid` | The amount the agent paid, as a `Decimal`. |
| `currency` | The currency the agent paid in. |
| `network` | The network the payment settled on. |
| `payment_method` | The payment method used. |

The payment is final when the view runs. If the view fails afterwards, retry the operation or refund the payment.
Free requests receive `None`.

## Error handling

| Error | Raised when |
| --- | --- |
| `MituConfigError` | The API key, base URL, currency, price or description is invalid, or a view has no request. |
| `MituApiError` | The Mitu API refuses a request in a way a retry will not fix, such as an invalid API key. Exposes `status`. |

Both derive from `MituError`.

## Development

```bash
python -m venv .venv && .venv/bin/pip install -e ".[dev]"
ruff check . && ruff format --check . && mypy --strict src && pytest
```

## License

[MIT](LICENSE)
