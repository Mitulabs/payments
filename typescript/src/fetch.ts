import { assertPrice, type Price } from './core/price.js'
import { withHeaders } from './core/response.js'
import type { PaymentContext } from './core/types.js'
import type { Mitu } from './mitu.js'

export type { Payment, PaymentContext } from './core/types.js'
export type { Price, PriceTerms } from './core/price.js'

/**
 * A Fetch API handler. `args` are whatever the runtime passes after the
 * request, such as `env` and `ctx` on Cloudflare Workers.
 */
export type FetchHandler<Args extends unknown[] = []> = (
  request: Request,
  context: PaymentContext,
  ...args: Args
) => Response | Promise<Response>

/**
 * Gates a Fetch API handler (Cloudflare Workers, Deno, Bun) behind a payment.
 *
 * @example
 * export default { fetch: charge(mitu, { amount: '0.01' })((request, { payment }, env, ctx) => ...) }
 */
export function charge(
  mitu: Mitu,
  price: Price<Request>,
): <Args extends unknown[] = []>(handler: FetchHandler<Args>) => (request: Request, ...args: Args) => Promise<Response> {
  assertPrice(price, mitu.defaultCurrency)
  return (handler) =>
    async (request, ...args) => {
      const admission = await mitu.admit(request, price)
      if (!admission.admitted) return admission.response
      return withHeaders(await handler(request, { payment: admission.payment }, ...args), admission.headers)
    }
}
