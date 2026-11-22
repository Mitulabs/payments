# @mitulabs/payments

Charge for API requests with machine payments. Mitu's TypeScript SDK puts any route behind a per-request payment that
AI agents and other clients pay with the [Machine Payments Protocol](https://mpp.dev) (MPP) or
[x402](https://x402.org).

## Installation

```bash
npm install @mitulabs/payments
```

Requires Node.js 22.12 or later, or a Fetch API runtime such as Cloudflare Workers, Deno or Bun.

## Quick start

```ts
import express from 'express'
import { createMitu } from '@mitulabs/payments'
import { charge } from '@mitulabs/payments/express'

const mitu = createMitu({ apiKey: process.env.MITU_API_KEY! })

const app = express()

app.get('/v1/search', charge(mitu, { amount: '0.05' }), (req, res) => {
  res.json({ results: search(req.query), payment: req.mitu?.payment })
})
```

An unpaid request receives `402 Payment Required` with MPP and x402 payment options. Once the client pays, the
handler runs and the response carries the payment receipt.

## Environments

The secret key selects the environment.

| Key | Environment |
| --- | --- |
| `sk_live_...` | Production |
| `sk_test_...` | Sandbox |

## Configuration

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `apiKey` | `string` | required | Mitu secret key. |
| `defaultCurrency` | `string` | `'USD'` | Currency for prices that do not specify one. |
| `logger` | `Logger` | none | Receives SDK logs at `debug`, `info`, `warn` and `error` levels. |
| `fetch` | `typeof fetch` | global `fetch` | Fetch implementation used for calls to Mitu. |

## Frameworks

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

app.get('/v1/search', charge(mitu, { amount: '0.05' }), (c) => c.json(search(c.req.query(), c.get('mitu').payment)))
```

### Next.js App Router

```ts
import { charge } from '@mitulabs/payments/next'

export const POST = charge(mitu, { amount: '0.50' })(async (request, { payment }) =>
  Response.json(await summarize(await request.json(), payment)),
)
```

### Elysia

```ts
import { charge, mituOf } from '@mitulabs/payments/elysia'

new Elysia().guard(charge(mitu, { amount: '0.05' }), (app) =>
  app.get('/v1/search', ({ request }) => search(request, mituOf(request)?.payment)),
)
```

### Cloudflare Workers, Deno and Bun

```ts
import { charge } from '@mitulabs/payments/fetch'

export default {
  fetch: charge(mitu, { amount: '0.05' })((request, { payment }) => Response.json(search(request, payment))),
}
```

### Other frameworks

`mitu.admit` works with any framework that exposes a Fetch API `Request`:

```ts
const admission = await mitu.admit(request, { amount: '0.05' })
if (!admission.admitted) return admission.response

const response = await handle(request, admission.payment)
for (const [name, value] of admission.headers) response.headers.set(name, value)
return response
```

## Pricing

Each route takes one price, set on the server:

```ts
charge(mitu, { amount: '0.05' })
charge(mitu, { amount: '2500', currency: 'NGN', description: 'Search' })
charge(mitu, (req) => (isPremium(req) ? { amount: '1.00' } : { amount: '0.10' }))
charge(mitu, (req) => (isFree(req) ? null : { amount: '0.05' }))
```

- Amounts are decimal strings greater than zero with up to two decimal places.
- A price function runs on the first request and on the paid retry, and must return the same price both times.
  Returning `null` serves the request free.
- Derive prices from server-side state, never from the request body.
- Prices in other currencies are converted at the rate locked when the payment is created.

## Payment

The handler receives the settled payment, or `null` for a free request:

| Field | Description |
| --- | --- |
| `id` | Payment ID, as sent in the `payment.completed` webhook |
| `reference` | Payment reference |
| `txHash` | Settlement transaction hash, when the payment method has one |
| `amountPaid` | Amount paid |
| `currency` | Currency of `amountPaid` |
| `network` | Settlement network, such as `base`, `tempo` or `solana` |
| `paymentMethod` | Payment method |

A settled payment is final. If the handler fails after payment, retry the work or refund the payment rather than
asking the client to pay again.

## Errors

| Error | When |
| --- | --- |
| `MituConfigError` | The API key, currency or a price is invalid. |
| `MituApiError` | Mitu rejects a request, for example because the key was revoked. Exposes `status` and `retryable`. |

If Mitu is temporarily unavailable, the route responds `503 Service Unavailable` with `Retry-After` and is not served
unpaid.

## License

[MIT](LICENSE)
