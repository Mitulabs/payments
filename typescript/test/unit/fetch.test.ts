import { describe, expect, it, vi } from 'vitest'
import { MituConfigError } from '../../src/core/errors.js'
import { charge, type FetchHandler } from '../../src/fetch.js'
import { fakeMitu, MPP_RECEIPT, mppCredential, problem, testMitu, x402Credential, X402_RECEIPT } from '../support/mitu.js'

const URL = 'https://api.shop.test/v1/search?q=jollof'

interface Env {
  region: string
}

function worker(settle?: Parameters<typeof fakeMitu>[0]) {
  const api = fakeMitu(settle)
  const handler = vi.fn<FetchHandler<[Env]>>((_, { payment }, env) => Response.json({ payment, region: env.region }, { headers: { 'x-app': '1' } }))
  const fetch = charge(testMitu(api), { amount: '0.05' })(handler)
  return { api, handler, fetch: (headers: Record<string, string> = {}) => fetch(new Request(URL, { headers }), { region: 'eu' }) }
}

describe('fetch charge', () => {
  it('answers Mitu’s 402 without running the handler', async () => {
    const { fetch, handler } = worker()

    const response = await fetch()

    expect(response.status).toBe(402)
    expect(response.headers.get('www-authenticate')).toMatch(/^Payment /)
    expect(handler).not.toHaveBeenCalled()
  })

  it.each([
    ['MPP', 'authorization', mppCredential, 'payment-receipt', MPP_RECEIPT],
    ['x402', 'payment-signature', x402Credential, 'payment-response', X402_RECEIPT],
  ] as const)('runs the handler with the %s payment and the runtime arguments, and adds the receipt', async (_, header, credentialOf, receiptHeader, receipt) => {
    const { fetch } = worker()
    const credential = await credentialOf(await fetch())

    const response = await fetch({ [header]: credential })

    expect(response.status).toBe(200)
    expect(response.headers.get(receiptHeader)).toBe(receipt)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(response.headers.get('x-app')).toBe('1')
    expect(await response.json()).toMatchObject({ region: 'eu', payment: { id: 'pmt_00000000000000000001' } })
  })

  it('passes Mitu’s refusal through without running the handler', async () => {
    const { fetch, handler } = worker({ settle: () => problem(410, 'payment-expired') })
    const credential = mppCredential(await fetch())

    const response = await fetch({ authorization: credential })

    expect(response.status).toBe(410)
    expect(handler).not.toHaveBeenCalled()
  })

  it('answers 503 with Retry-After while Mitu is down', async () => {
    const { fetch, handler } = worker({ apiDown: 'unreachable' })

    const response = await fetch()

    expect(response.status).toBe(503)
    expect(response.headers.get('retry-after')).toBe('5')
    expect(handler).not.toHaveBeenCalled()
  })

  it('serves a free route without calling Mitu or touching the response', async () => {
    const api = fakeMitu()
    const ok = Response.json({ ok: true })
    const handler = charge(testMitu(api), null)((_, { payment }) => (payment ? Response.error() : ok))

    expect(await handler(new Request(URL))).toBe(ok)
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('refuses an invalid fixed price at registration', () => {
    const mitu = testMitu(fakeMitu())

    expect(() => charge(mitu, { amount: '-1' })).toThrow(MituConfigError)
    expect(() => charge(mitu, { amount: '1', currency: 'U$' })).toThrow(MituConfigError)
  })
})
