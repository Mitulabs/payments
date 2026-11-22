import { Hono } from 'hono'
import { describe, expect, it, vi } from 'vitest'
import { charge, type MituEnv } from '../../src/hono.js'
import { fakeMitu, mppCredential, problem, testMitu, x402Credential, X402_RECEIPT } from '../support/mitu.js'

function app(options?: Parameters<typeof fakeMitu>[0]) {
  const api = fakeMitu(options)
  const handler = vi.fn((payment: unknown) => payment)
  const hono = new Hono<MituEnv>().get(
    '/v1/search',
    charge(testMitu(api), (c) => (c.req.query('q') === 'free' ? null : { amount: '0.05', currency: 'USD' })),
    (c) => c.json({ payment: handler(c.get('mitu').payment) }),
  )
  const get = (query = '', headers: Record<string, string> = {}) => hono.request(`/v1/search${query}`, { headers })
  return { api, handler, get }
}

describe('hono charge', () => {
  it('answers Mitu’s 402 without running the handler', async () => {
    const { get, handler } = app()

    const response = await get()

    expect(response.status).toBe(402)
    expect(response.headers.get('www-authenticate')).toMatch(/^Payment /)
    expect(handler).not.toHaveBeenCalled()
  })

  it('runs the handler with the payment on c.get("mitu") and adds the receipt', async () => {
    const { get } = app()
    const credential = await x402Credential(await get())

    const response = await get('', { 'payment-signature': credential })

    expect(response.status).toBe(200)
    expect(response.headers.get('payment-response')).toBe(X402_RECEIPT)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(await response.json()).toMatchObject({ payment: { id: 'pmt_00000000000000000001', paymentMethod: 'x402' } })
  })

  it('prices from the context, serving a free request with a null payment', async () => {
    const { get, api } = app()

    const response = await get('?q=free')

    expect(await response.json()).toEqual({ payment: null })
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('passes Mitu’s refusal through without running the handler', async () => {
    const { get, handler } = app({ settle: () => problem(409, 'already-paid') })
    const credential = mppCredential(await get())

    expect((await get('', { authorization: credential })).status).toBe(409)
    expect(handler).not.toHaveBeenCalled()
  })

  it('answers 503 while Mitu is down', async () => {
    const { get } = app({ apiDown: 503 })

    const response = await get()

    expect(response.status).toBe(503)
    expect(response.headers.get('retry-after')).toBe('5')
  })
})
