import type { Request, RequestHandler, Response } from 'express'
import { CREDENTIAL_HEADERS } from './core/credential.js'
import { assertPrice, resolvePrice, type Price } from './core/price.js'
import type { PaymentContext } from './core/types.js'
import type { Mitu } from './mitu.js'

export type { Payment, PaymentContext } from './core/types.js'
export type { Price, PriceTerms } from './core/price.js'

declare module 'express-serve-static-core' {
  interface Request {
    /** Set by `charge()` before the route handler runs. */
    mitu?: PaymentContext
  }
}

/**
 * Express middleware that gates the route behind a payment, then calls the
 * next handler with the settled payment on `req.mitu.payment`.
 *
 * @example
 * app.get('/v1/search', charge(mitu, { amount: '0.01' }), (req, res) => res.json(search(req.mitu?.payment)))
 */
export function charge(mitu: Mitu, price: Price<Request>): RequestHandler {
  assertPrice(price, mitu.defaultCurrency)
  return async (req, res, next) => {
    const admission = await mitu.admit(fetchRequestOf(req), await resolvePrice(price, req, mitu.defaultCurrency))
    if (!admission.admitted) {
      await send(res, admission.response)
      return
    }
    for (const [name, value] of admission.headers) res.setHeader(name, value)
    req.mitu = { payment: admission.payment }
    next()
  }
}

/** The parts of the request the payment flow reads: its method, full path and credential headers. */
function fetchRequestOf(req: Request): globalThis.Request {
  const headers = new Headers()
  for (const name of CREDENTIAL_HEADERS) {
    const value = req.get(name)
    if (value) headers.set(name, value)
  }
  return new globalThis.Request(new URL(req.originalUrl, `${req.protocol}://${req.get('host') ?? 'localhost'}`), {
    method: req.method,
    headers,
  })
}

async function send(res: Response, response: globalThis.Response): Promise<void> {
  res.status(response.status)
  for (const [name, value] of response.headers) res.setHeader(name, value)
  res.end(new Uint8Array(await response.arrayBuffer()))
}
