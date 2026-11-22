# Mitu Payments for Go

Charge for API requests with machine payments. Mitu's Go SDK puts any route behind a per-request payment that AI agents
and other clients pay with the [Machine Payments Protocol](https://mpp.dev) (MPP) or [x402](https://x402.org).

## Installation

```bash
go get github.com/mitulabs/payments/go
```

Requires Go 1.26 or later. The core module depends only on the standard library.

## Quick start

```go
import mitu "github.com/mitulabs/payments/go"

m, err := mitu.New(ctx, mitu.Options{APIKey: os.Getenv("MITU_API_KEY")})
if err != nil {
	log.Fatal(err)
}

mux.Handle("GET /v1/search", m.Charge(mitu.Price{Amount: "0.05"})(http.HandlerFunc(search)))

func search(w http.ResponseWriter, r *http.Request) {
	payment, _ := mitu.PaymentFromContext(r.Context())
	// ...
}
```

An unpaid request receives `402 Payment Required` with MPP and x402 payment options. Once the client pays, the handler
runs and the response carries the payment receipt.

## Environments

The secret key selects the environment.

| Key | Environment |
| --- | --- |
| `sk_live_...` | Production |
| `sk_test_...` | Sandbox |

## Configuration

| Field | Default | Description |
| --- | --- | --- |
| `APIKey` | required | Mitu secret key. |
| `DefaultCurrency` | `"USD"` | Currency for prices that do not specify one. |
| `HTTPClient` | default client | Client used for calls to Mitu. |
| `Logger` | discard | `*slog.Logger` for SDK logs. |

`New` returns an error wrapping `ErrConfig` when the options are invalid.

## Frameworks

`Charge` returns standard `net/http` middleware, which also works with chi. Gin, Echo and Fiber have adapters in their
own modules, so the core module never depends on a web framework.

```go
import mitugin "github.com/mitulabs/payments/go/gin"

router.GET("/v1/search", mitugin.Charge(m, mitu.Price{Amount: "0.05"}), search)
```

| Framework | Module | Package |
| --- | --- | --- |
| Gin | `github.com/mitulabs/payments/go/gin` | `mitugin` |
| Echo v5 | `github.com/mitulabs/payments/go/echo` | `mituecho` |
| Fiber v3 | `github.com/mitulabs/payments/go/fiber` | `mitufiber` |

## Pricing

```go
mitu.Price{Amount: "0.05"}
mitu.Price{Amount: "2500", Currency: "NGN", Description: "Search"}
mitu.Price{AmountFunc: func(r *http.Request) (string, error) {
	if isPremium(r) {
		return "1.00", nil
	}
	return "0.10", nil
}}
```

- Set exactly one of `Amount` and `AmountFunc`. Amounts are decimal strings greater than zero with up to two decimal
  places.
- `AmountFunc` runs on the first request and on the paid retry, and must return the same amount both times. Returning
  `""` serves the request free.
- Derive prices from server-side state, never from the request body.
- Prices in other currencies are converted at the rate locked when the payment is created.
- `Charge` panics at route registration if a price or its currency is invalid.

## Payment

`PaymentFromContext` returns the settled payment and `true`, or `false` for a free request:

| Field | Description |
| --- | --- |
| `ID` | Payment ID, as sent in the `payment.completed` webhook |
| `Reference` | Payment reference |
| `TxHash` | Settlement transaction hash, when the payment method has one |
| `AmountPaid` | Amount paid |
| `Currency` | Currency of `AmountPaid` |
| `Network` | Settlement network |
| `PaymentMethod` | Payment method |

A settled payment is final. If the handler fails after payment, retry the work or refund the payment rather than
asking the client to pay again.

## Errors

If Mitu is temporarily unavailable, the route responds `503 Service Unavailable` with `Retry-After` and is not served
unpaid. If Mitu rejects a request, for example because the key was revoked, the route responds
`500 Internal Server Error` and logs the cause.

## License

[MIT](LICENSE)
