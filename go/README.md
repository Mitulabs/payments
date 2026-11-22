# Mitu Payments SDK for Go

Go SDK for monetizing HTTP APIs with machine payments. It requires payment per request from AI agents through the
[Machine Payments Protocol](https://mpp.dev) (MPP) and [x402](https://x402.org), using Mitu's existing payment flow.

Mitu issues the payment challenges, settles each payment on chain as the facilitator, locks the exchange rate when the
payment is created, and notifies the merchant with the `payment.completed` webhook. The SDK creates a Mitu payment for
each charged request, relays Mitu's `402 Payment Required` response to the agent, and serves the route once Mitu has
settled the agent's payment. It does not verify payments, hold receiving addresses or quote exchange rates.

## Requirements

- Go 1.26 or later
- No dependencies outside the standard library

## Installation

```bash
go get github.com/mitulabs/payments/go
```

## Configuration

```go
import mitu "github.com/mitulabs/payments/go"

m, err := mitu.New(ctx, mitu.Options{
	APIKey:          os.Getenv("MITU_API_KEY"),
	DefaultCurrency: "NGN",
})
if err != nil {
	log.Fatal(err)
}
```

`New` validates the options and makes no network call, so a service starts even when Mitu is unreachable. Invalid
options return an error wrapping `ErrConfig`.

### Options

| Field | Default | Description |
| --- | --- | --- |
| `APIKey` | required | Mitu secret key. `sk_test_` selects the sandbox, `sk_live_` production. Sent only to the Mitu API. |
| `DefaultCurrency` | `"USD"` | Currency for prices that do not specify one. |
| `BaseURL` | by key | Mitu API origin. Must use HTTPS unless it points at the local machine. |
| `HTTPClient` | default | Client for calls to Mitu. Redirects are never followed; every call times out after 10 seconds. |
| `Logger` | discard | `*slog.Logger` for SDK logs. |

## Usage

### net/http and chi

```go
mux.Handle("GET /v1/search", m.Charge(mitu.Price{Amount: "0.50", Currency: "USD"})(http.HandlerFunc(search)))

func search(w http.ResponseWriter, r *http.Request) {
	payment, _ := mitu.PaymentFromContext(r.Context())
	// ...
}
```

### Gin, Echo and Fiber

Each framework adapter is a separate module, so the core module does not depend on any web framework:

```go
import mitugin "github.com/mitulabs/payments/go/gin"

router.GET("/v1/search", mitugin.Charge(m, mitu.Price{Amount: "0.50", Currency: "USD"}), search)
```

| Framework | Module | Package |
| --- | --- | --- |
| Gin | `github.com/mitulabs/payments/go/gin` | `mitugin` |
| Echo v5 | `github.com/mitulabs/payments/go/echo` | `mituecho` |
| Fiber v3 | `github.com/mitulabs/payments/go/fiber` | `mitufiber` |

## Request flow

1. **Unpaid request.** The SDK creates a Mitu payment for the route's price, bound to the request's method and path
   and valid for 15 minutes, and returns Mitu's `402` response with its MPP (`WWW-Authenticate`) and x402
   (`PAYMENT-REQUIRED`) challenges.
2. **Paid retry.** The agent retries with its credential. The SDK reads the Mitu payment the credential answers and
   checks that it belongs to the merchant and was created for this method, path, amount and currency. A credential
   that fails this check, or cannot be read, receives a fresh `402`.
3. **Settlement.** The SDK forwards the credential, and only the credential, to the payment's `agent_url`. Mitu
   settles the payment and returns its receipt. The handler then runs, and Mitu's receipt headers (`Payment-Receipt`
   for MPP, `PAYMENT-RESPONSE` for x402) are added to its response.
4. **Refusal.** If Mitu refuses the payment, for example a replayed credential (`403`) or an expired or already paid
   payment (`409`, `410`), its status, protocol headers and problem body are returned unchanged and the handler does
   not run.

Responses that carry payment state are sent with `Cache-Control: no-store`.

## Pricing

Set exactly one of `Price.Amount` and `Price.AmountFunc`. Amounts are decimal strings greater than zero with at most
two decimal places, such as `"5000"` or `"12.50"`; money never passes through floating point.

`Price.AmountFunc` computes the amount from the request. It is evaluated on the initial request and on the paid retry
and must return the same amount for both; it must never read the amount from the request body. Returning `""` serves
the request without payment, and returning an error responds `500 Internal Server Error`.

`Charge` panics at route registration if a price or its currency is invalid.

## Payments

`PaymentFromContext` returns the settled `Payment` for the request: Mitu's payment `ID` and `Reference`, `TxHash`,
`AmountPaid`, `Currency`, `Network` and `PaymentMethod`. It reports `false` for a request served without payment.
Use Mitu's `payment.completed` webhook as the system of record for fulfilment and reconciliation.

## Availability

If Mitu cannot be reached, or answers with a server error, the route responds `503 Service Unavailable` with
`Retry-After: 5` and is never served unpaid. If Mitu refuses the SDK's own request, for example an invalid API key,
the route responds `500 Internal Server Error` and logs the cause.

## Security

- The API key is sent only to the Mitu API as `x-api-key`. It is never sent to `agent_url`, logged or included in
  errors.
- Only the payment credential header is forwarded to `agent_url`; cookies and other request headers are not.
- Only Mitu's protocol and receipt headers are copied back to the agent.
- Logs never contain keys, credentials, receipts or bodies.

Report vulnerabilities to security@mitu.ai.

## License

[MIT](LICENSE)
