# mitulabs-payments

Charge for API requests with machine payments. Mitu's Python SDK puts any route behind a per-request payment that AI
agents and other clients pay with the [Machine Payments Protocol](https://mpp.dev) (MPP) or [x402](https://x402.org).

## Installation

```bash
pip install "mitulabs-payments[fastapi]"
```

Use the `flask` or `django` extra for those frameworks. Requires Python 3.12 or later.

## Quick start

```python
import os

from fastapi import FastAPI, Request
from mitulabs_payments import Mitu

mitu = Mitu(os.environ["MITU_API_KEY"])
app = FastAPI()


@app.get("/v1/search")
@mitu.charge(amount="0.05")
async def search(request: Request):
    return {"results": run_search(request.query_params), "payment": request.state.mitu_payment}
```

An unpaid request receives `402 Payment Required` with MPP and x402 payment options. Once the client pays, the view
runs and the response carries the payment receipt.

## Environments

The secret key selects the environment.

| Key | Environment |
| --- | --- |
| `sk_live_...` | Production |
| `sk_test_...` | Sandbox |

## Configuration

| Argument | Default | Description |
| --- | --- | --- |
| `api_key` | required | Mitu secret key. |
| `default_currency` | `"USD"` | Currency for prices that do not specify one. |

Call `await mitu.close()`, or `mitu.close_sync()` in synchronous applications, at shutdown.

## Frameworks

`@mitu.charge` goes below the framework's route decorator. Sync and async views are supported.

### FastAPI

```python
@app.get("/v1/search")
@mitu.charge(amount="0.05")
async def search(request: Request):
    return run_search(request.query_params, request.state.mitu_payment)
```

Charged FastAPI routes take `request: Request`.

### Flask

```python
from flask import g, request


@app.get("/v1/search")
@mitu.charge(amount="0.05")
def search():
    return run_search(request.args, g.mitu_payment)
```

### Django

```python
@mitu.charge(amount="0.05")
def search(request):
    return JsonResponse(run_search(request.GET, request.mitu_payment))
```

## Pricing

Each route takes one price:

```python
mitu.charge(amount="0.05")
mitu.charge(amount="2500", currency="NGN", description="Search")
mitu.charge(amount=lambda request: "1.00" if is_premium(request) else "0.10")
mitu.charge(amount=lambda request: None if is_free(request) else "0.05")
```

- Amounts are decimal strings greater than zero with up to two decimal places.
- A price function runs on the first request and on the paid retry, and must return the same price both times.
  Returning `None` serves the request free. On async views it may be async.
- Derive prices from server-side state, never from the request body.
- Prices in other currencies are converted at the rate locked when the payment is created.

## Payment

The view receives the settled payment, or `None` for a free request:

| Field | Description |
| --- | --- |
| `payment_id` | Payment ID, as sent in the `payment.completed` webhook |
| `reference` | Payment reference |
| `tx_hash` | Settlement transaction hash, when the payment method has one |
| `amount_paid` | Amount paid, as a `Decimal` |
| `currency` | Currency of `amount_paid` |
| `network` | Settlement network |
| `payment_method` | Payment method |

| Framework | Where |
| --- | --- |
| FastAPI | `request.state.mitu_payment` |
| Flask | `g.mitu_payment` |
| Django | `request.mitu_payment` |

A settled payment is final. If the view fails after payment, retry the work or refund the payment rather than asking
the client to pay again.

## Errors

| Error | When |
| --- | --- |
| `MituConfigError` | The API key, currency or a price is invalid. |
| `MituApiError` | Mitu rejects a request, for example because the key was revoked. Exposes `status`. |

Both derive from `MituError`. If Mitu is temporarily unavailable, the route responds `503 Service Unavailable` with
`Retry-After` and is not served unpaid.

## License

[MIT](LICENSE)
