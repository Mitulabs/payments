import type { Context } from 'elysia'
import { assertPrice, type Price } from './core/price.js'
import type { PaymentContext } from './core/types.js'
import type { Mitu } from './mitu.js'

export type { Payment, PaymentContext } from './core/types.js'
export type { Price, PriceTerms } from './core/price.js'

/** The part of Elysia's request context the hook reads. */
export interface HookContext {
  request: Request
  set: Pick<Context['set'], 'headers'>
}

/** The lifecycle hook for `.guard()`, which scopes it to the guarded routes. */
export interface ChargeHooks {
  beforeHandle: (context: HookContext) => Promise<Response | undefined>
}

const contexts = new WeakMap<Request, PaymentContext>()

/**
 * The payment `charge()` settled for this request, or `undefined` when the
 * route is not behind `charge()`.
 *
 * @example
 * app.get('/v1/search', ({ request }) => search(mituOf(request)?.payment))
 */
export function mituOf(request: Request): PaymentContext | undefined {
  return contexts.get(request)
}

/**
 * Elysia hooks that gate the routes of a `.guard()` behind a payment. The
 * handler reads the settled payment with `mituOf(request)`.
 *
 * @example
 * new Elysia().guard(charge(mitu, { amount: '0.01' }), (app) => app.get('/v1/search', handler))
 */
export function charge(mitu: Mitu, price: Price<Request>): ChargeHooks {
  assertPrice(price, mitu.defaultCurrency)
  return {
    async beforeHandle({ request, set }) {
      const admission = await mitu.admit(request, price)
      if (!admission.admitted) return admission.response
      for (const [name, value] of admission.headers) set.headers[name] = value
      contexts.set(request, { payment: admission.payment })
      return undefined
    },
  }
}
