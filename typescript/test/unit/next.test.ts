import { describe, expect, it, vi } from 'vitest'
import { charge, type PaidRouteHandler } from '../../src/next.js'
import { fakeMitu, MPP_RECEIPT, mppCredential, problem, testMitu } from '../support/mitu.js'

type Params = { id: string }

function route(options?: Parameters<typeof fakeMitu>[0]) {
  const api = fakeMitu(options)
  const handler = vi.fn<PaidRouteHandler<Params>>(async (_, { params, payment }) => Response.json({ id: (await params).id, payment }))
  const POST = charge(testMitu(api), { amount: '0.05' })(handler)
  const post = (headers: Record<string, string> = {}) =>
    POST(new Request('https://shop.test/api/reports/42', { method: 'POST', headers }), { params: Promise.resolve({ id: '42' }) })
  return { api, handler, post }
}

describe('next charge', () => {
  it('answers Mitu’s 402 without running the handler', async () => {
    const { post, handler } = route()

    const response = await post()

    expect(response.status).toBe(402)
    expect(response.headers.get('payment-required')).toBeTruthy()
    expect(handler).not.toHaveBeenCalled()
  })

  it('runs the handler with its params and the payment, and adds the receipt', async () => {
    const { post, api } = route()
    const credential = mppCredential(await post())

    const response = await post({ authorization: credential })

    expect(response.status).toBe(200)
    expect(response.headers.get('payment-receipt')).toBe(MPP_RECEIPT)
    expect(await response.json()).toMatchObject({ id: '42', payment: { id: 'pmt_00000000000000000001', txHash: expect.any(String) } })
    expect(api.payments.get('pmt_00000000000000000001')?.metadata).toMatchObject({ method: 'POST', path: '/api/reports/42' })
  })

  it('passes a replayed credential’s 403 through without running the handler', async () => {
    const { post, handler } = route({ settle: () => problem(403, 'credential-replayed') })
    const credential = mppCredential(await post())

    expect((await post({ authorization: credential })).status).toBe(403)
    expect(handler).not.toHaveBeenCalled()
  })

  it('answers 503 while Mitu is down', async () => {
    const { post } = route({ apiDown: 500 })

    expect((await post()).status).toBe(503)
  })
})
