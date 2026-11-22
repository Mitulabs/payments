import { MituConfigError } from './errors.js'

/** Mitu takes amounts above zero with at most two decimal places. */
const AMOUNT = /^(\d+)(?:\.(\d{1,2}))?$/
const CURRENCY = /^[A-Za-z]{3,10}$/

export interface PriceTerms {
  /** A decimal string above zero with at most two decimal places, such as `'0.05'` or `'2500'`. */
  amount: string
  /** Defaults to the `defaultCurrency` given to `createMitu`. */
  currency?: string
  /** Shown to the agent in the payment challenge. Defaults to the method and path. */
  description?: string
}

/**
 * A route's price: fixed terms, `null` to serve the route free, or a function
 * of the request returning either. A function is called on the first request
 * and on the paid retry, and must return the same terms both times. Derive it
 * from server state, never from the request body.
 */
export type Price<Input> = PriceTerms | null | ((input: Input) => PriceTerms | null | Promise<PriceTerms | null>)

export interface ResolvedPrice {
  amount: string
  currency: string
  description?: string
}

export async function resolvePrice<Input>(price: Price<Input>, input: Input, defaultCurrency: string): Promise<ResolvedPrice | null> {
  const terms = typeof price === 'function' ? await price(input) : price
  return terms && checkPrice(terms, defaultCurrency)
}

/** Fails at route registration, rather than on the first request, for a fixed price Mitu would refuse. */
export function assertPrice<Input>(price: Price<Input>, defaultCurrency: string): void {
  if (price && typeof price !== 'function') checkPrice(price, defaultCurrency)
}

export function checkPrice({ amount, currency, description }: PriceTerms, defaultCurrency: string): ResolvedPrice {
  if (!AMOUNT.test(amount) || !/[1-9]/.test(amount)) {
    throw new MituConfigError(`Price amount ${JSON.stringify(amount)} must be above zero with at most two decimal places`)
  }
  return { amount, currency: currencyCode(currency ?? defaultCurrency), ...(description && { description }) }
}

export function currencyCode(currency: string): string {
  if (!CURRENCY.test(currency)) {
    throw new MituConfigError(`Currency ${JSON.stringify(currency)} is not a currency code, such as "USD" or "NGN"`)
  }
  return currency.toUpperCase()
}

/** Whether two decimal strings name the same amount, so `'5'` matches Mitu's `'5.00'`. */
export function sameAmount(a: string, b: string): boolean {
  const x = cents(a)
  return x !== null && x === cents(b)
}

function cents(amount: string): bigint | null {
  const match = AMOUNT.exec(amount)
  return match ? BigInt((match[1] ?? '') + (match[2] ?? '').padEnd(2, '0')) : null
}
