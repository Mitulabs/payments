import type { Context, MiddlewareHandler } from 'hono'
import { assertPrice, resolvePrice, type Price } from './core/price.js'
import type { PaymentContext } from './core/types.js'
import type { Mitu } from './mitu.js'

export type { Payment, PaymentContext } from './core/types.js'
export type { Price, PriceTerms } from './core/price.js'

/** The context variables `charge()` sets, read in the handler with `c.get('mitu')`. */
export interface MituEnv {
  Variables: { mitu: PaymentContext }
}

/**
 * Hono middleware that gates the route behind a payment, then runs the next
 * handler with the settled payment on `c.get('mitu').payment`.
 *
 * @example
 * app.get('/v1/search', charge(mitu, { amount: '0.01' }), (c) => c.json(search(c.get('mitu').payment)))
 */
export function charge(mitu: Mitu, price: Price<Context<MituEnv, string>>): MiddlewareHandler<MituEnv> {
  assertPrice(price, mitu.defaultCurrency)
  return async (c, next) => {
    const admission = await mitu.admit(c.req.raw, await resolvePrice(price, c, mitu.defaultCurrency))
    if (!admission.admitted) return admission.response
    c.set('mitu', { payment: admission.payment })
    await next()
    for (const [name, value] of admission.headers) c.header(name, value)
  }
}
