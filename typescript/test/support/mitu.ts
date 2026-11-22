import { x402Client, x402HTTPClient } from '@x402/core/client'
import { ExactEvmScheme } from '@x402/evm/exact/client'
import { Challenge, Credential } from 'mppx'
import { vi } from 'vitest'
import { isRecord } from '../../src/core/guards.js'
import type { Logger } from '../../src/core/logger.js'
import { createMitu, type CreateMituOptions } from '../../src/mitu.js'

export const API_KEY = 'sk_test_0123456789abcdef0123456789abcdef'
export const BASE_URL = 'https://api.mitu.test/economy'
export const AGENT_ORIGIN = 'https://agent.mitu.test'
export const TX_HASH = '0x5e771ed'
export const MPP_RECEIPT = 'mpp-receipt-value'
export const X402_RECEIPT = 'eDQwMi1yZWNlaXB0'

const SECRET_KEY = 'challenge-secret-key-for-tests-only'
const USDC_BASE = '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913'
const PAY_TO = '0x000000000000000000000000000000000000dEaD'

export interface StoredPayment {
  id: string
  amount: string
  currency: string
  reference: string
  description: string
  expires_at: string
  metadata: Record<string, string>
}

export interface Call {
  method: string
  url: URL
  headers: Headers
  body: unknown
}

/** What `agent_url` answers to a credential: Mitu's settlement outcome. */
export type Settlement = (payment: StoredPayment, headers: Headers) => Response

/** Mitu's 200 for a settled payment, with the receipt header of the protocol the credential used. */
export const paid: Settlement = (payment, headers) =>
  Response.json(
    {
      status: 'paid',
      resource_type: 'payment',
      resource_id: payment.id,
      receipt: {
        transaction_id: 'txn_1',
        tx_hash: TX_HASH,
        amount_paid: '0.05',
        currency: 'USDC',
        network: 'base',
        payment_method: headers.has('authorization') ? 'mpp' : 'x402',
        confirmed_at: '2026-11-22T10:00:00.000Z',
      },
    },
    {
      headers: headers.has('authorization')
        ? { 'payment-receipt': MPP_RECEIPT }
        : { 'payment-response': X402_RECEIPT, 'x-payment-response': X402_RECEIPT },
    },
  )

/** A Mitu problem response, as `/pay/:id` answers a refused credential. */
export function problem(status: number, slug: string, headers: Record<string, string> = {}): Response {
  return Response.json(
    { type: `https://paymentauth.org/problems/${slug}`, status, detail: 'secret problem detail' },
    { status, headers: { 'content-type': 'application/problem+json', ...headers } },
  )
}

export interface FakeMituOptions {
  settle?: Settlement
  /** Fails every call to the Mitu API, as when it is down. */
  apiDown?: 'unreachable' | number
}

/**
 * The Mitu API and its `agent_url` behind one fake `fetch`. The 402 carries a
 * real mppx challenge scoped to the payment id, as Mitu issues it,
 * and an x402 v2 `PAYMENT-REQUIRED` whose resource is the `agent_url`.
 */
export function fakeMitu({ settle = paid, apiDown }: FakeMituOptions = {}) {
  const payments = new Map<string, StoredPayment>()
  const calls: Call[] = []
  let agentDown: 'unreachable' | number | undefined

  const fetch = vi.fn(async (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
    const url = urlOf(input)
    const method = init?.method ?? 'GET'
    const headers = new Headers(init?.headers)
    const body: unknown = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
    calls.push({ method, url, headers, body })

    if (url.href.startsWith(BASE_URL)) {
      if (apiDown === 'unreachable') throw new TypeError('fetch failed')
      if (apiDown) return new Response('down', { status: apiDown })
      return api(method, url.pathname.slice(new URL(BASE_URL).pathname.length), body)
    }
    if (url.origin === AGENT_ORIGIN) {
      if (agentDown === 'unreachable') throw new TypeError('fetch failed')
      if (agentDown) return new Response('down', { status: agentDown })
      const payment = payments.get(url.pathname.split('/').at(-1) ?? '')
      if (!payment) return problem(404, 'not-found')
      const credential = headers.has('authorization') || headers.has('payment-signature') || headers.has('x-payment')
      return credential ? settle(payment, headers) : challenge(payment)
    }
    throw new TypeError(`offline: ${url.origin}`)
  })

  function api(method: string, path: string, body: unknown): Response {
    if (method === 'POST' && path === '/v1/public/payments' && isRecord(body)) {
      const payment: StoredPayment = {
        id: `pmt_${String(payments.size + 1).padStart(20, '0')}`,
        amount: Number(body.amount).toFixed(2),
        currency: String(body.currency).toUpperCase(),
        reference: String(body.reference),
        description: String(body.description),
        expires_at: String(body.expires_at),
        metadata: isRecord(body.metadata) ? Object.fromEntries(Object.entries(body.metadata).map(([k, v]) => [k, String(v)])) : {},
      }
      payments.set(payment.id, payment)
      return Response.json({ message: 'Payment created', data: view(payment) }, { status: 201 })
    }
    const id = /^\/v1\/public\/payments\/([^/]+)$/.exec(path)?.[1]
    const payment = id && payments.get(decodeURIComponent(id))
    if (method === 'GET' && payment) return Response.json({ message: 'Payment retrieved', data: view(payment) })
    return Response.json({ message: 'Payment not found', error: 'Not Found' }, { status: 404 })
  }

  return {
    fetch,
    calls,
    payments,
    /** Adds a payment as another integration would, outside this SDK. */
    store(payment: Omit<StoredPayment, 'id'>): StoredPayment {
      const stored = { ...payment, id: `pmt_${String(payments.size + 1).padStart(20, '0')}` }
      payments.set(stored.id, stored)
      return stored
    },
    /** Fails every later call to `agent_url`. */
    failAgentUrl(mode: 'unreachable' | number) {
      agentDown = mode
    },
    agentCalls: () => calls.filter((call) => call.url.origin === AGENT_ORIGIN),
    apiCalls: () => calls.filter((call) => call.url.href.startsWith(BASE_URL)),
  }
}

export type FakeMitu = ReturnType<typeof fakeMitu>

function urlOf(input: string | URL | Request): URL {
  return new URL(input instanceof Request ? input.url : input)
}

export function view(payment: StoredPayment) {
  return {
    ...payment,
    object: 'payment',
    status: 'pending',
    prices: null,
    customer_id: null,
    tenant_id: null,
    amount_paid: null,
    paid_with: null,
    paid_at: null,
    url: `https://mitu.test/pay/${payment.id}`,
    agent_url: `${AGENT_ORIGIN}/pay/${payment.id}`,
    created_at: '2026-11-22T10:00:00.000Z',
  }
}

/** An MPP charge challenge bound to a payment id through mppx's `scope`, as Mitu issues it. */
function mppChallenge(paymentId: string) {
  return Challenge.from({
    secretKey: SECRET_KEY,
    realm: 'api.mitu.ai',
    method: 'tempo',
    intent: 'charge',
    request: { amount: '50000', currency: '0x20C000000000000000000000b9537d11c60E8b50', recipient: PAY_TO },
    meta: { _mppx_scope: paymentId },
  })
}

function challenge(payment: StoredPayment): Response {
  const mpp = mppChallenge(payment.id)
  const required = {
    x402Version: 2,
    resource: { url: `${AGENT_ORIGIN}/pay/${payment.id}`, description: payment.description, mimeType: 'application/json' },
    accepts: [
      {
        scheme: 'exact',
        network: 'eip155:8453',
        amount: '50000',
        asset: USDC_BASE,
        payTo: PAY_TO,
        maxTimeoutSeconds: 300,
        extra: { name: 'USD Coin', version: '2' },
      },
    ],
  }
  return Response.json(
    { type: 'https://paymentauth.org/problems/payment-required', status: 402, resource_id: payment.id },
    {
      status: 402,
      headers: {
        'content-type': 'application/problem+json',
        'www-authenticate': Challenge.serialize(mpp),
        'payment-required': btoa(JSON.stringify(required)),
        'set-cookie': 'mitu_session=secret',
        'x-internal': 'not-for-agents',
      },
    },
  )
}

/** The `Authorization` value an mppx client sends in answer to the 402's MPP challenge. */
export function mppCredential(response: Response): string {
  const challenge = Challenge.deserialize(response.headers.get('www-authenticate') ?? '')
  return Credential.serialize({ challenge, payload: { type: 'hash', hash: TX_HASH } })
}

/** An MPP credential answering a challenge for any payment id. */
export function mppCredentialFor(paymentId: string): string {
  return Credential.serialize({ challenge: mppChallenge(paymentId), payload: { type: 'hash', hash: TX_HASH } })
}

/** The `PAYMENT-SIGNATURE` value an x402 client sends in answer to the 402's `PAYMENT-REQUIRED`. */
export async function x402Credential(response: Response): Promise<string> {
  const signer = {
    address: '0x1111111111111111111111111111111111111111',
    signTypedData: () => Promise.resolve(`0x${'ab'.repeat(65)}` as const),
  } as const
  const client = new x402Client().register('eip155:*', new ExactEvmScheme(signer))
  const http = new x402HTTPClient(client)
  const required = http.getPaymentRequiredResponse((name) => response.headers.get(name))
  const headers = http.encodePaymentSignatureHeader(await client.createPaymentPayload(required))
  return headers['PAYMENT-SIGNATURE'] ?? ''
}

export function recordingLogger() {
  const lines: string[] = []
  const record = (message: string, fields?: Record<string, unknown>) => {
    lines.push(`${message} ${JSON.stringify(fields ?? {})}`)
  }
  const logger: Logger = { debug: record, info: record, warn: record, error: record }
  return { logger, lines }
}

export function testMitu(mitu: FakeMitu, options: Partial<CreateMituOptions> = {}) {
  return createMitu({ apiKey: API_KEY, baseUrl: BASE_URL, fetch: mitu.fetch, ...options })
}
