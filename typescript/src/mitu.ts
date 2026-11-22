import { callAgentUrl, challengeOf, refusalOf, settled, unavailable } from './core/agent.js'
import { readCredential, type Credential } from './core/credential.js'
import { apiUrlFor, assertApiKey } from './core/environment.js'
import { MituApiError } from './core/errors.js'
import { createHttpClient, PACKAGE_NAME, PACKAGE_VERSION } from './core/http.js'
import { silentLogger, type Logger } from './core/logger.js'
import { createPayments, type PaymentRecord } from './core/payments.js'
import { currencyCode, resolvePrice, sameAmount, type Price, type ResolvedPrice } from './core/price.js'
import type { Admission } from './core/types.js'

/** How long an agent has to pay a challenge. */
const PAYMENT_TTL_MS = 15 * 60_000

export interface CreateMituOptions {
  /** Mitu secret key, `sk_live_...` or `sk_test_...`. Sent only to the Mitu API. */
  apiKey: string
  /** The currency of a price that names none. Defaults to `USD`. */
  defaultCurrency?: string
  /** Receives the package's logs, which never contain keys, credentials, receipts or bodies. */
  logger?: Logger
  /** Replaces the global `fetch` for calls to Mitu. */
  fetch?: typeof fetch
}

export interface Mitu {
  readonly defaultCurrency: string
  /**
   * Gates one request behind Mitu's payment flow: the framework-neutral core
   * the adapters are built on. Without a credential, Mitu's 402 for a new
   * payment; with one, Mitu settles it and the request is admitted with the
   * payment. A `null` price admits the request free, without calling Mitu.
   */
  admit(request: Request, price: Price<Request>): Promise<Admission>
}

interface Route {
  method: string
  path: string
}

export function createMitu({ apiKey, defaultCurrency = 'USD', logger = silentLogger, fetch = globalThis.fetch }: CreateMituOptions): Mitu {
  assertApiKey(apiKey)
  const currency = currencyCode(defaultCurrency)
  const payments = createPayments(createHttpClient({ apiKey, baseUrl: apiUrlFor(apiKey), fetch }))

  async function challenge(price: ResolvedPrice, route: Route): Promise<Response> {
    const payment = await payments.create({
      amount: price.amount,
      currency: price.currency,
      reference: newReference(),
      description: price.description ?? `${route.method} ${route.path}`,
      expiresAt: new Date(Date.now() + PAYMENT_TTL_MS),
      metadata: { sdk: `${PACKAGE_NAME}@${PACKAGE_VERSION}`, ...route },
    })
    logger.debug('Payment created', { paymentId: payment.id, ...route })
    return challengeOf(await callAgentUrl(fetch, payment.agentUrl))
  }

  async function settle(record: PaymentRecord, credential: Credential, route: Route): Promise<Admission> {
    const upstream = await callAgentUrl(fetch, record.agentUrl, credential.headers)
    if (upstream.status !== 200) {
      logger.info('Mitu refused the payment credential', { paymentId: record.id, status: upstream.status, ...route })
      return { admitted: false, response: await refusalOf(upstream) }
    }
    logger.info('Payment settled', { paymentId: record.id, ...route })
    return { admitted: true, ...(await settled(upstream, record)) }
  }

  async function gate(price: ResolvedPrice, request: Request): Promise<Admission> {
    const route = { method: request.method, path: new URL(request.url).pathname }
    const credential = readCredential(request.headers)
    if (credential) {
      const record = credential.paymentId ? await payments.get(credential.paymentId) : null
      if (record && pays(record, price, route)) return settle(record, credential, route)
      logger.info('Payment credential is not for this route and price', route)
    }
    return { admitted: false, response: await challenge(price, route) }
  }

  return {
    defaultCurrency: currency,
    async admit(request, price) {
      const resolved = await resolvePrice(price, request, currency)
      if (!resolved) return { admitted: true, payment: null, headers: new Headers() }
      try {
        return await gate(resolved, request)
      } catch (error) {
        if (!(error instanceof MituApiError && error.retryable)) throw error
        logger.warn('Mitu is unavailable', { status: error.status })
        return { admitted: false, response: unavailable() }
      }
    },
  }
}

/** A payment pays for a request only at this price and for this route: a cheaper one for another route must not unlock it. */
function pays(record: PaymentRecord, price: ResolvedPrice, route: Route): boolean {
  return (
    sameAmount(price.amount, record.amount) &&
    record.currency.toUpperCase() === price.currency &&
    record.metadata.method === route.method &&
    record.metadata.path === route.path
  )
}

/** Unguessable, so no one can predict a reference and front-run its payment. */
function newReference(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return `sdk_${Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')}`
}
