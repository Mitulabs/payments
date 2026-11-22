# @mitulabs/payments

TypeScript SDK for monetizing HTTP APIs with machine payments. It requires payment per request through the
[Machine Payments Protocol](https://mpp.dev) (MPP) and [x402](https://x402.org), and settles proceeds through Mitu in
the merchant's currency.

The SDK is a thin layer over Mitu's payment flow. Mitu issues the payment challenges, verifies and settles each payment
on chain, locks the exchange rate when the payment is created and sends the `payment.completed` webhook. The SDK puts
that flow in front of your routes: it creates a payment for each unpaid request, returns Mitu's challenge to the agent,
and serves the request once Mitu has settled the agent's payment. Payment verification, receiving addresses and
exchange rates stay with Mitu.

## Requirements

- Node.js 22.12 or later, or a Fetch API runtime (Cloudflare Workers, Deno, Bun)
- A Mitu secret key (`sk_live_...` or `sk_test_...`)

The package has no runtime dependencies. Framework adapters use the framework you already have installed.

## Installation

```bash
npm install @mitulabs/payments
```

## Configuration

```ts
import { createMitu } from '@mitulabs/payments'

export const mitu = createMitu({
  apiKey: process.env.MITU_API_KEY!,
  defaultCurrency: 'USD',
})
```

### Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `apiKey` | `string` | required | Mitu secret key. Sent only to the Mitu API, never to agents and never logged. |
| `defaultCurrency` | `string` | `'USD'` | Currency code for prices that do not specify one. |
| `baseUrl` | `string` | `https://api.mitu.ai/economy` | Mitu API base URL. Must use HTTPS unless it points at localhost. |
| `logger` | `Logger` | silent | Receives SDK logs (`debug`, `info`, `warn`, `error`). |
| `fetch` | `typeof fetch` | global | Fetch implementation for calls to Mitu. |

## Usage

Each framework adapter takes the `Mitu` instance and a price, and exposes the settled payment to the handler.

### Express

```ts
import { charge } from '@mitulabs/payments/express'

app.get('/v1/search', charge(mitu, { amount: '0.05' }), (req, res) => {
  res.json(search(req.query, req.mitu?.payment))
})
```

### Hono

```ts
import { charge, type MituEnv } from '@mitulabs/payments/hono'

const app = new Hono<MituEnv>()

app.get('/v1/search', charge(mitu, { amount: '0.05' }), (c) =>
  c.json(search(c.req.query(), c.get('mitu').payment)),
)
```

### Next.js App Router

```ts
import { charge } from '@mitulabs/payments/next'

export const POST = charge(mitu, { amount: '0.50' })(async (request, { params, payment }) =>
  Response.json(await generate((await params).id, await request.json(), payment)),
)
```

### Elysia

```ts
import { charge, mituOf } from '@mitulabs/payments/elysia'

new Elysia().guard(charge(mitu, { amount: '0.05' }), (app) =>
  app.get('/v1/search', ({ request }) => search(request, mituOf(request)?.payment)),
)
```

### Fetch API

```ts
import { charge } from '@mitulabs/payments/fetch'

export default {
  fetch: charge(mitu, { amount: '0.05' })((request, { payment }, env, ctx) =>
    Response.json(search(request, payment)),
  ),
}
```

### Other frameworks

`mitu.admit(request, price)` is the framework-neutral core the adapters are built on. It takes a Fetch API `Request`
and returns either a response to send as is, or the payment and the receipt headers to add to your response:

```ts
const admission = await mitu.admit(request, { amount: '0.05' })
if (!admission.admitted) return admission.response

const response = await handle(request, admission.payment)
for (const [name, value] of admission.headers) response.headers.set(name, value)
return response
```

## Pricing

```ts
{ amount: '0.05' }                                          // fixed, default currency
{ amount: '2500', currency: 'NGN', description: 'Search' }  // fixed, local currency
(req) => (req.query.tier === 'pro' ? { amount: '1.00' } : { amount: '0.10' })  // per request
null                                                        // free
```

Amounts are decimal strings above zero with at most two decimal places. A price function receives the framework's
request (the Express `req`, the Hono context, or the Fetch `Request`) and is evaluated on the initial request and on the
paid retry; it must return the same price for both. Returning `null` serves the request without payment and without
calling Mitu. Prices must be derived from server-side state, never from the request body.

Fixed prices are validated when the route is registered. Prices in a local currency are converted by Mitu at the rate
locked when the payment is created.

## Payment flow

For each request to a charged route:

1. **No payment credential.** The SDK creates a Mitu payment for the route's price, valid for 15 minutes and bound to
   the request's method and path, and returns Mitu's `402 Payment Required` with its MPP (`WWW-Authenticate`) and x402
   (`PAYMENT-REQUIRED`) challenges.
2. **Payment credential.** The agent retries with an MPP credential (`Authorization` or `Payment-Authorization`) or an
   x402 payment (`PAYMENT-SIGNATURE` or `X-PAYMENT`). The SDK reads the Mitu payment it answers and confirms that the
   payment belongs to this merchant and matches the route, amount and currency; otherwise it returns a new challenge.
   It then forwards the credential, and only the credential, to Mitu, which verifies and settles the payment.
3. **Settled.** The handler runs with the payment, and Mitu's receipt (`Payment-Receipt` or `PAYMENT-RESPONSE`) is
   added to the response. If Mitu refuses the credential (for example a replay, an expired payment or a failed
   verification), its response is returned unchanged and the handler does not run.

Responses that carry payment state are sent with `Cache-Control: no-store`. If Mitu cannot be reached, the route
responds `503 Service Unavailable` with `Retry-After: 5` and is never served unpaid.

## Payments

Handlers receive a `Payment`, or `null` when the request was served without payment:

| Field | Description |
| --- | --- |
| `id` | Mitu payment ID, as in the `payment.completed` webhook |
| `reference` | Unique reference the SDK assigned to the payment |
| `txHash` | Settlement transaction hash, or `null` |
| `amountPaid` | Amount the agent paid, in `currency` |
| `currency` | Currency of `amountPaid`, e.g. `USDC` |
| `network` | Network the payment settled on, e.g. `base`, `tempo` or `solana` |
| `paymentMethod` | Payment method as reported by Mitu |

A settled payment is final. If the handler fails after payment, retry the operation or issue a refund; do not request
payment again. Persist results keyed by `id` to make fulfilment idempotent. Proceeds settle through Mitu, which
notifies your webhook endpoint of each completed payment.

## Error handling

| Error | Raised when |
| --- | --- |
| `MituConfigError` | Options or a price are invalid, at startup, route registration or, for a price function, at request time |
| `MituApiError` | Mitu rejects a request in a way retrying cannot fix, such as a revoked key. Exposes `status` and `retryable` |

Unavailability is not raised: the adapters respond `503` with `Retry-After`. Error messages and logs never include
the API key, payment credentials, receipts or request and response bodies.

## Development

```bash
npm ci
npm run lint && npm run typecheck && npm test && npm run build
```

Unit tests run without network access: a fake `fetch` stands in for the Mitu API and the agent payment endpoint, and
credentials are built with the official `mppx` and `@x402/core` clients.

## License

[MIT](LICENSE)
