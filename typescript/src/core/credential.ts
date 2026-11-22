import { isRecord } from './guards.js'

/** Every request header that can carry a payment credential, by its lowercase name. */
export const CREDENTIAL_HEADERS = ['authorization', 'payment-authorization', 'payment-signature', 'x-payment'] as const

/** mppx refuses credentials longer than this before decoding them; so does the SDK. */
const MAX_LENGTH = 64 * 1024

/** The reserved challenge `meta` key mppx binds a challenge's `scope` to: Mitu's payment id. */
const SCOPE_KEY = '_mppx_scope'

const PAYMENT_SCHEME = /^Payment\s+/i
const PAYMENT_ID = /^pmt_[0-9a-z]{1,46}$/
const AGENT_PATH = /\/pay\/([^/]+)\/?$/

export interface Credential {
  /** The headers to send to the payment's `agent_url`: the credential and nothing else. */
  headers: Record<string, string>
  /** The Mitu payment the credential answers a challenge for, or `null` when it names none. */
  paymentId: string | null
}

/**
 * The payment credential a request carries, MPP first as Mitu reads it: an
 * MPP `Payment` credential in `Authorization` or `Payment-Authorization`, or
 * an x402 payment in `PAYMENT-SIGNATURE` or `X-PAYMENT`. The payment id it
 * names is unverified; Mitu decides whether the credential pays it.
 */
export function readCredential(headers: Headers): Credential | null {
  const mpp = paymentScheme(headers.get('authorization')) ?? paymentScheme(headers.get('payment-authorization'))
  if (mpp) return { headers: { authorization: mpp }, paymentId: mppPaymentId(mpp) }

  for (const name of ['payment-signature', 'x-payment'] as const) {
    const value = headers.get(name)?.trim()
    if (value && value.length <= MAX_LENGTH) return { headers: { [name]: value }, paymentId: x402PaymentId(value) }
  }
  return null
}

/** The `Payment` scheme of an `Authorization` value that may list several schemes. */
function paymentScheme(value: string | null): string | null {
  if (!value || value.length > MAX_LENGTH) return null
  return value
    .split(',')
    .map((scheme) => scheme.trim())
    .find((scheme) => PAYMENT_SCHEME.test(scheme)) ?? null
}

/**
 * An MPP credential is base64url JSON echoing the challenge it answers. Mitu's
 * challenges carry the payment id as mppx's scope, in `opaque`: base64url JSON
 * of the challenge `meta`. Older mppx clients echo `opaque` as the object itself.
 */
function mppPaymentId(scheme: string): string | null {
  const wire = decodeJson(scheme.replace(PAYMENT_SCHEME, ''))
  const challenge = isRecord(wire) ? wire.challenge : undefined
  if (!isRecord(challenge)) return null
  const meta = typeof challenge.opaque === 'string' ? decodeJson(challenge.opaque) : challenge.opaque
  return isRecord(meta) ? paymentIdOf(meta[SCOPE_KEY]) : null
}

/**
 * An x402 v2 payment is base64 JSON echoing the challenge's `resource`, whose
 * URL Mitu sets to the payment's `agent_url`, `.../pay/<payment id>`. A v1
 * payment carries no resource, and so names no payment.
 */
function x402PaymentId(value: string): string | null {
  const payload = decodeJson(value)
  const resource = isRecord(payload) ? payload.resource : undefined
  const url = isRecord(resource) && typeof resource.url === 'string' ? URL.parse(resource.url) : null
  return paymentIdOf(url && AGENT_PATH.exec(url.pathname)?.[1])
}

function paymentIdOf(value: unknown): string | null {
  return typeof value === 'string' && PAYMENT_ID.test(value) ? value : null
}

/** Decodes base64 or base64url JSON, padded or not. */
function decodeJson(encoded: string): unknown {
  try {
    const base64 = encoded.trim().replaceAll('-', '+').replaceAll('_', '/')
    const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, '='))
    const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
    return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown
  } catch {
    return undefined
  }
}
