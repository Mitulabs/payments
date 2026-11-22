import { MituApiError } from './errors.js'
import { isRecord } from './guards.js'
import { readJson, TIMEOUT_MS, USER_AGENT } from './http.js'
import type { PaymentRecord } from './payments.js'
import type { Payment } from './types.js'

/** Headers of Mitu's answers an agent needs: the challenges, and when to retry. */
const RELAYED_HEADERS = ['www-authenticate', 'payment-required', 'retry-after', 'content-type'] as const

/** Mitu's receipts: MPP's, then x402's under its v2 and v1 names. */
const RECEIPT_HEADERS = ['payment-receipt', 'payment-response', 'x-payment-response'] as const

const NO_STORE = { 'cache-control': 'no-store' } as const

/**
 * Calls a payment's `agent_url` as the agent would: with no credential for
 * its challenge, or with the agent's credential to settle it. Only the given
 * headers are sent; never the merchant's key or the request's cookies.
 */
export async function callAgentUrl(fetch: typeof globalThis.fetch, agentUrl: string, credential: Record<string, string> = {}): Promise<Response> {
  try {
    return await fetch(agentUrl, {
      method: 'GET',
      redirect: 'manual',
      signal: AbortSignal.timeout(TIMEOUT_MS),
      headers: { accept: 'application/json', 'user-agent': USER_AGENT, ...credential },
    })
  } catch (error) {
    throw new MituApiError(0, 'Mitu pay endpoint unreachable', { cause: error })
  }
}

/** The 402 for a new payment. Anything but a client error means Mitu cannot take the payment now. */
export function challengeOf(upstream: Response): Promise<Response> {
  return relay(upstream, upstream.status >= 400 && upstream.status < 500)
}

/** Mitu's refusal of a credential, a 402, 403, 409, 410 or 503 problem, for the agent as it is. */
export function refusalOf(upstream: Response): Promise<Response> {
  return relay(upstream, upstream.status >= 400)
}

/** Mitu's status, protocol headers and body, never cached. Any other answer, such as a redirect, is not Mitu's to relay. */
async function relay(upstream: Response, relayable: boolean): Promise<Response> {
  if (!relayable) {
    await upstream.body?.cancel()
    throw new MituApiError(upstream.status, `Mitu pay endpoint answered ${upstream.status}`, { retryable: true })
  }
  const headers = new Headers(NO_STORE)
  for (const name of RELAYED_HEADERS) {
    const value = upstream.headers.get(name)
    if (value) headers.set(name, value)
  }
  return new Response(await upstream.arrayBuffer(), { status: upstream.status, headers })
}

/** Mitu settled the payment: the payment for the handler, and the receipt headers for the response. */
export async function settled(upstream: Response, record: PaymentRecord): Promise<{ payment: Payment; headers: Headers }> {
  const body = await readJson(upstream)
  if (!isRecord(body) || body.status !== 'paid' || body.resource_id !== record.id) {
    throw new MituApiError(upstream.status, 'Mitu pay endpoint answered 200 without settling the payment', { retryable: true })
  }
  const receipt = isRecord(body.receipt) ? body.receipt : {}
  const payment: Payment = {
    id: record.id,
    reference: record.reference,
    txHash: stringOrNull(receipt.tx_hash),
    amountPaid: stringOrNull(receipt.amount_paid) ?? record.amount,
    currency: stringOrNull(receipt.currency) ?? record.currency,
    network: stringOrNull(receipt.network),
    paymentMethod: stringOrNull(receipt.payment_method),
  }

  const headers = new Headers(NO_STORE)
  for (const name of RECEIPT_HEADERS) {
    const value = upstream.headers.get(name)
    if (value) headers.set(name, value)
  }
  return { payment, headers }
}

export function unavailable(): Response {
  return new Response('Service Unavailable', {
    status: 503,
    headers: { ...NO_STORE, 'content-type': 'text/plain; charset=utf-8', 'retry-after': '5' },
  })
}

function stringOrNull(value: unknown): string | null {
  return typeof value === 'string' && value ? value : null
}
