import { MituApiError } from './errors.js'
import { isRecord } from './guards.js'
import type { HttpClient } from './http.js'

/** The fields of a Mitu payment the SDK reads. */
export interface PaymentRecord {
  id: string
  amount: string
  currency: string
  reference: string
  metadata: Record<string, string>
  /** Where an agent pays it, over MPP or x402. */
  agentUrl: string
}

export interface CreatePayment {
  amount: string
  currency: string
  reference: string
  description: string
  expiresAt: Date
  metadata: Record<string, string>
}

export interface Payments {
  create(payment: CreatePayment): Promise<PaymentRecord>
  /** The merchant's own payment, or `null` when Mitu knows no payment of this merchant by that id. */
  get(id: string): Promise<PaymentRecord | null>
}

export function createPayments(http: HttpClient): Payments {
  return {
    async create({ amount, currency, reference, description, expiresAt, metadata }) {
      const body = { amount: Number(amount), currency, reference, description, expires_at: expiresAt.toISOString(), metadata }
      return paymentOf(await http.request({ method: 'POST', path: '/v1/payments', body }))
    },
    async get(id) {
      try {
        return paymentOf(await http.request({ method: 'GET', path: `/v1/payments/${encodeURIComponent(id)}` }))
      } catch (error) {
        if (error instanceof MituApiError && error.status === 404) return null
        throw error
      }
    },
  }
}

function paymentOf(payload: unknown): PaymentRecord {
  const data = isRecord(payload) ? payload.data : undefined
  if (isRecord(data)) {
    const { id, amount, currency, reference, metadata, agent_url } = data
    const agentUrl = typeof agent_url === 'string' ? URL.parse(agent_url) : null
    if (
      typeof id === 'string' &&
      typeof amount === 'string' &&
      typeof currency === 'string' &&
      typeof reference === 'string' &&
      agentUrl?.protocol === 'https:'
    ) {
      return { id, amount, currency, reference, metadata: stringsOf(metadata), agentUrl: agentUrl.href }
    }
  }
  throw new MituApiError(0, 'Mitu API returned a payment it could not read', { retryable: true })
}

function stringsOf(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {}
  return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === 'string'))
}
