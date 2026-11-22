import { assertPrice, type Price } from './core/price.js'
import { withHeaders } from './core/response.js'
import type { PaymentContext } from './core/types.js'
import type { Mitu } from './mitu.js'

export type { Payment, PaymentContext } from './core/types.js'
export type { Price, PriceTerms } from './core/price.js'

export type RouteParams = Record<string, string | string[] | undefined>

/** The second argument Next.js passes to an App Router route handler. */
export interface RouteContext<Params = RouteParams> {
  params: Promise<Params>
}

export type RouteHandler<Params = RouteParams> = (request: Request, context: RouteContext<Params>) => Promise<Response>

/** A route handler that also receives the settled payment, or `null` when the request was free. */
export type PaidRouteHandler<Params = RouteParams> = (
  request: Request,
  context: RouteContext<Params> & PaymentContext,
) => Response | Promise<Response>

/**
 * Gates a Next.js App Router route handler behind a payment.
 *
 * @example
 * export const POST = charge(mitu, { amount: '0.05' })(async (request, { params, payment }) => ...)
 */
export function charge(mitu: Mitu, price: Price<Request>): <Params = RouteParams>(handler: PaidRouteHandler<Params>) => RouteHandler<Params> {
  assertPrice(price, mitu.defaultCurrency)
  return (handler) => async (request, context) => {
    const admission = await mitu.admit(request, price)
    if (!admission.admitted) return admission.response
    return withHeaders(await handler(request, { ...context, payment: admission.payment }), admission.headers)
  }
}
