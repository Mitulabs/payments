import type { Server } from 'node:http'
import express, { type ErrorRequestHandler } from 'express'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Price } from '../../src/core/price.js'
import { charge } from '../../src/express.js'
import { fakeMitu, MPP_RECEIPT, mppCredential, problem, testMitu, x402Credential, X402_RECEIPT } from '../support/mitu.js'

let server: Server | undefined

afterEach(async () => {
  const running = server
  server = undefined
  if (running) await new Promise((resolve) => running.close(resolve))
})

/** Serves one charged route on this machine and returns a client for it. */
async function serve(price: Price<express.Request> = { amount: '0.05' }, options?: Parameters<typeof fakeMitu>[0]) {
  const api = fakeMitu(options)
  const handler = vi.fn((req: express.Request, res: express.Response) => {
    res.json({ payment: req.mitu?.payment ?? null })
  })
  const onError = vi.fn<ErrorRequestHandler>((_error, _req, res, _next) => {
    res.status(500).end()
  })
  const app = express()
  app.get('/v1/items/:id', charge(testMitu(api), price), handler)
  app.use(onError)

  const listening = app.listen(0, '127.0.0.1')
  server = listening
  await new Promise((resolve) => listening.once('listening', resolve))
  const address = listening.address()
  if (!address || typeof address === 'string') throw new Error('server has no port')
  const get = (headers: Record<string, string> = {}) => fetch(`http://127.0.0.1:${address.port}/v1/items/7?full=1`, { headers })
  return { api, handler, onError, get }
}

describe('express charge', () => {
  it('answers Mitu’s 402 without running the handler', async () => {
    const { get, handler, api } = await serve()

    const response = await get()

    expect(response.status).toBe(402)
    expect(response.headers.get('www-authenticate')).toMatch(/^Payment /)
    expect(response.headers.get('payment-required')).toBeTruthy()
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(await response.json()).toMatchObject({ status: 402 })
    expect(handler).not.toHaveBeenCalled()
    expect(api.payments.get('pmt_00000000000000000001')?.metadata).toMatchObject({ method: 'GET', path: '/v1/items/7' })
  })

  it.each([
    ['MPP', 'authorization', mppCredential, 'payment-receipt', MPP_RECEIPT],
    ['x402', 'payment-signature', x402Credential, 'payment-response', X402_RECEIPT],
  ] as const)('runs the handler with the %s payment on req.mitu and adds the receipt', async (_, header, credentialOf, receiptHeader, receipt) => {
    const { get } = await serve()
    const credential = await credentialOf(await get())

    const response = await get({ [header]: credential })

    expect(response.status).toBe(200)
    expect(response.headers.get(receiptHeader)).toBe(receipt)
    expect(await response.json()).toMatchObject({ payment: { id: 'pmt_00000000000000000001', network: 'base' } })
  })

  it('prices from the request, serving a free request with a null payment', async () => {
    const { get, api } = await serve((req) => (req.params.id === '7' ? null : { amount: '1' }))

    expect(await (await get()).json()).toEqual({ payment: null })
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('passes Mitu’s refusal through without running the handler', async () => {
    const { get, handler } = await serve(undefined, { settle: () => problem(410, 'payment-expired') })
    const credential = mppCredential(await get())

    const response = await get({ authorization: credential })

    expect(response.status).toBe(410)
    expect(await response.json()).toMatchObject({ type: 'https://paymentauth.org/problems/payment-expired' })
    expect(handler).not.toHaveBeenCalled()
  })

  it('answers 503 with Retry-After while Mitu is down', async () => {
    const { get, handler } = await serve(undefined, { apiDown: 502 })

    const response = await get()

    expect(response.status).toBe(503)
    expect(response.headers.get('retry-after')).toBe('5')
    expect(handler).not.toHaveBeenCalled()
  })

  it('hands an error from a price function to the app’s error handler', async () => {
    const { get, onError, handler } = await serve(() => ({ amount: 'free' }))

    expect((await get()).status).toBe(500)
    expect(onError).toHaveBeenCalled()
    expect(handler).not.toHaveBeenCalled()
  })
})
