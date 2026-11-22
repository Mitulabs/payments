import { Elysia } from 'elysia'
import { describe, expect, it, vi } from 'vitest'
import { charge, mituOf } from '../../src/elysia.js'
import type { Price } from '../../src/core/price.js'
import { fakeMitu, MPP_RECEIPT, mppCredential, problem, testMitu } from '../support/mitu.js'

function app(price: Price<Request> = { amount: '0.05' }, options?: Parameters<typeof fakeMitu>[0]) {
  const api = fakeMitu(options)
  const handler = vi.fn(({ request }: { request: Request }) => ({ payment: mituOf(request)?.payment }))
  const elysia = new Elysia()
    .guard(charge(testMitu(api), price), (group) =>
      group.post('/orders', handler).post('/raw', ({ request }) => new Response(mituOf(request)?.payment?.id ?? 'free')),
    )
    .get('/open', ({ request }) => ({ context: mituOf(request) ?? 'none' }))
  const post = (path: string, headers: Record<string, string> = {}) =>
    elysia.handle(new Request(`http://localhost${path}`, { method: 'POST', headers }))
  return { api, handler, elysia, post }
}

describe('elysia charge', () => {
  it('answers Mitu’s 402 without running the handler', async () => {
    const { post, handler } = app()

    const response = await post('/orders')

    expect(response.status).toBe(402)
    expect(response.headers.get('www-authenticate')).toMatch(/^Payment /)
    expect(handler).not.toHaveBeenCalled()
  })

  it.each(['/orders', '/raw'])('runs %s with the payment from mituOf(request) and adds the receipt', async (path) => {
    const { post } = app()
    const credential = mppCredential(await post(path))

    const response = await post(path, { authorization: credential })

    expect(response.status).toBe(200)
    expect(response.headers.get('payment-receipt')).toBe(MPP_RECEIPT)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(await response.text()).toContain('pmt_00000000000000000001')
  })

  it('serves a free request with a null payment', async () => {
    const { post, api } = app(null)

    expect(await (await post('/orders')).json()).toEqual({ payment: null })
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('leaves routes outside the guard alone', async () => {
    const { elysia, api } = app()

    const response = await elysia.handle(new Request('http://localhost/open'))

    expect(await response.json()).toEqual({ context: 'none' })
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('passes Mitu’s refusal through without running the handler', async () => {
    const { post, handler } = app(undefined, { settle: () => problem(403, 'credential-replayed') })
    const credential = mppCredential(await post('/orders'))

    expect((await post('/orders', { authorization: credential })).status).toBe(403)
    expect(handler).not.toHaveBeenCalled()
  })

  it('answers 503 while Mitu is down', async () => {
    const { post } = app(undefined, { apiDown: 'unreachable' })

    expect((await post('/orders')).status).toBe(503)
  })
})
