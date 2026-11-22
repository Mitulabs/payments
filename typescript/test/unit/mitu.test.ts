import { Credential } from 'mppx'
import { describe, expect, it } from 'vitest'
import { MituApiError, MituConfigError } from '../../src/core/errors.js'
import { isRecord } from '../../src/core/guards.js'
import { PACKAGE_VERSION } from '../../src/core/http.js'
import type { PriceTerms } from '../../src/core/price.js'
import type { Admission } from '../../src/core/types.js'
import { createMitu, type Mitu } from '../../src/mitu.js'
import {
  AGENT_ORIGIN,
  API_KEY,
  BASE_URL,
  fakeMitu,
  MPP_RECEIPT,
  mppCredential,
  mppCredentialFor,
  problem,
  recordingLogger,
  testMitu,
  TX_HASH,
  X402_RECEIPT,
  x402Credential,
  type FakeMitu,
} from '../support/mitu.js'

const URL = 'https://shop.test/v1/search'
const price: PriceTerms = { amount: '0.05', currency: 'usd', description: 'Search' }

const get = (headers: Record<string, string> = {}, url = URL) => new Request(url, { headers })

const decode = (base64url: string): unknown => JSON.parse(Buffer.from(base64url, 'base64url').toString())

function refused(admission: Admission): Response {
  if (admission.admitted) throw new Error('expected the request to be refused')
  return admission.response
}

/** The 402 of a first, unpaid request, from which the agent builds its credential. */
async function firstChallenge(mitu: Mitu, url = URL, terms = price): Promise<Response> {
  return refused(await mitu.admit(get({}, url), terms))
}

describe('an unpaid request', () => {
  it('creates a payment for the route and passes Mitu’s 402 through with its headers', async () => {
    const api = fakeMitu()
    const before = Date.now()

    const response = await firstChallenge(testMitu(api), `${URL}?q=lagos`)

    const [create, challenge] = api.calls
    expect(create?.method).toBe('POST')
    expect(create?.url.href).toBe(`${BASE_URL}/v1/public/payments`)
    expect(create?.headers.get('x-api-key')).toBe(API_KEY)
    expect(create?.body).toMatchObject({
      amount: 0.05,
      currency: 'USD',
      description: 'Search',
      reference: expect.stringMatching(/^sdk_[0-9a-f]{32}$/),
      metadata: { sdk: `@mitulabs/payments@${PACKAGE_VERSION}`, method: 'GET', path: '/v1/search' },
    })
    const ttl = Date.parse(api.payments.get('pmt_00000000000000000001')?.expires_at ?? '') - before
    expect(ttl).toBeGreaterThanOrEqual(15 * 60_000)
    expect(ttl).toBeLessThan(15 * 60_000 + 5_000)

    expect(challenge?.url.href).toBe(`${AGENT_ORIGIN}/pay/pmt_00000000000000000001`)
    expect(challenge?.headers.has('authorization')).toBe(false)

    expect(response.status).toBe(402)
    expect(response.headers.get('www-authenticate')).toMatch(/^Payment /)
    expect(response.headers.get('payment-required')).toBeTruthy()
    expect(response.headers.get('content-type')).toBe('application/problem+json')
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(response.headers.has('set-cookie')).toBe(false)
    expect(response.headers.has('x-internal')).toBe(false)
    expect(await response.json()).toMatchObject({ status: 402, resource_id: 'pmt_00000000000000000001' })
  })

  it('gives every challenge a fresh, unguessable reference', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)

    await firstChallenge(mitu)
    await firstChallenge(mitu)

    const [first, second] = [...api.payments.values()].map((payment) => payment.reference)
    expect(first).not.toBe(second)
  })

  it('describes the payment by method and path when the price has no description', async () => {
    const api = fakeMitu()

    await firstChallenge(testMitu(api), URL, { amount: '2500', currency: 'NGN' })

    expect(api.calls[0]?.body).toMatchObject({ amount: 2500, currency: 'NGN', description: 'GET /v1/search' })
  })

  it('prices in the default currency', async () => {
    const api = fakeMitu()

    await firstChallenge(testMitu(api, { defaultCurrency: 'ngn' }), URL, { amount: '2500' })

    expect(api.calls[0]?.body).toMatchObject({ currency: 'NGN' })
  })
})

describe('a paid request', () => {
  it.each([
    ['MPP', 'authorization', mppCredential, 'payment-receipt', MPP_RECEIPT],
    ['x402', 'payment-signature', x402Credential, 'payment-response', X402_RECEIPT],
  ] as const)('settles a real %s credential at agent_url and admits the request', async (_, header, credentialOf, receiptHeader, receipt) => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = await credentialOf(await firstChallenge(mitu))
    api.calls.length = 0

    const admission = await mitu.admit(get({ [header]: credential, cookie: 'session=secret' }), price)

    const [lookup, settle] = api.calls
    expect(lookup?.method).toBe('GET')
    expect(lookup?.url.href).toBe(`${BASE_URL}/v1/public/payments/pmt_00000000000000000001`)
    expect(settle?.url.href).toBe(`${AGENT_ORIGIN}/pay/pmt_00000000000000000001`)
    expect(settle?.headers.get(header)).toBe(credential)
    expect(settle?.headers.has('x-api-key')).toBe(false)
    expect(settle?.headers.has('cookie')).toBe(false)

    expect(admission).toMatchObject({
      admitted: true,
      payment: {
        id: 'pmt_00000000000000000001',
        reference: expect.stringMatching(/^sdk_/),
        txHash: TX_HASH,
        amountPaid: '0.05',
        currency: 'USDC',
        network: 'base',
      },
    })
    if (!admission.admitted) return
    expect(admission.headers.get(receiptHeader)).toBe(receipt)
    expect(admission.headers.get('cache-control')).toBe('no-store')
  })

  it('reads an MPP credential from Payment-Authorization and sends it to Mitu as Authorization', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    const admission = await mitu.admit(get({ authorization: 'Bearer merchant-session', 'payment-authorization': credential }), price)

    expect(admission.admitted).toBe(true)
    expect(api.agentCalls().at(-1)?.headers.get('authorization')).toBe(credential)
  })

  it('takes the Payment scheme alone from an Authorization header listing several', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    const admission = await mitu.admit(get({ authorization: `Bearer merchant-session, ${credential}` }), price)

    expect(admission.admitted).toBe(true)
    expect(api.agentCalls().at(-1)?.headers.get('authorization')).toBe(credential)
  })

  it('reads the payment id from the opaque object older mppx clients echo', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const wire = decode(mppCredential(await firstChallenge(mitu)).slice('Payment '.length))
    if (!isRecord(wire) || !isRecord(wire.challenge)) throw new Error('not an MPP credential')
    const { challenge } = wire
    const opaque = decode(String(challenge.opaque))
    const legacy = `Payment ${Buffer.from(JSON.stringify({ ...wire, challenge: { ...challenge, opaque } })).toString('base64url')}`
    expect(Credential.deserialize(legacy).challenge.meta).toEqual({ _mppx_scope: 'pmt_00000000000000000001' })

    expect((await mitu.admit(get({ authorization: legacy }), price)).admitted).toBe(true)
  })

  it('matches an amount however its decimals are written', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu, URL, { amount: '5', currency: 'USD' }))

    expect(api.payments.get('pmt_00000000000000000001')?.amount).toBe('5.00')
    expect((await mitu.admit(get({ authorization: credential }), { amount: '5.0', currency: 'USD' })).admitted).toBe(true)
  })
})

describe('a credential that does not pay for this request', () => {
  async function retried(api: FakeMitu, mitu: Mitu, request: Request, terms = price) {
    api.calls.length = 0
    const response = refused(await mitu.admit(request, terms))
    expect(response.status).toBe(402)
    expect(api.calls.map((call) => `${call.method} ${call.url.pathname}`)).toContain('POST /economy/v1/public/payments')
    expect(api.agentCalls().every((call) => !call.headers.has('authorization') && !call.headers.has('payment-signature'))).toBe(true)
  }

  it('gets a fresh 402 when it paid for another route', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu, 'https://shop.test/v1/cheap'))

    await retried(api, mitu, get({ authorization: credential }))
  })

  it('gets a fresh 402 when it paid for another method on the same path', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = await x402Credential(await firstChallenge(mitu))

    await retried(api, mitu, new Request(URL, { method: 'POST', headers: { 'payment-signature': credential } }))
  })

  it('gets a fresh 402 when the price has changed', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    await retried(api, mitu, get({ authorization: credential }), { ...price, amount: '0.50' })
  })

  it('gets a fresh 402 when the currency has changed', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    await retried(api, mitu, get({ authorization: credential }), { ...price, currency: 'NGN' })
  })

  it('gets a fresh 402 for a payment made outside the SDK', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const other = api.store({
      amount: '0.05',
      currency: 'USD',
      reference: 'ORDER-1',
      description: 'An order',
      expires_at: '2026-11-23T10:00:00.000Z',
      metadata: {},
    })

    await retried(api, mitu, get({ authorization: mppCredentialFor(other.id) }))
  })

  it('gets a fresh 402 for a payment id Mitu does not know for this merchant', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))
    api.payments.clear()

    await retried(api, mitu, get({ authorization: credential }))
  })

  it.each([
    ['an MPP credential that is not base64url JSON', { authorization: 'Payment not-a-credential' }],
    ['Mitu’s own key="value" credential', { authorization: 'Payment method="mpp_crypto", tx_hash="0xabc"' }],
    ['an MPP credential without a scope', { authorization: `Payment ${Buffer.from('{"challenge":{"id":"x"}}').toString('base64url')}` }],
    ['an x402 payment that is not base64 JSON', { 'payment-signature': '%%%' }],
    ['an x402 v1 payment, which names no resource', { 'x-payment': btoa('{"x402Version":1,"scheme":"exact","network":"base","payload":{}}') }],
    ['an x402 payment for another resource', { 'payment-signature': btoa('{"x402Version":2,"resource":{"url":"https://elsewhere.test/item/1"}}') }],
    ['an x402 payment naming a malformed id', { 'payment-signature': btoa('{"x402Version":2,"resource":{"url":"https://agent.mitu.test/pay/..%2Fsecrets"}}') }],
    ['an oversized credential', { authorization: `Payment ${'a'.repeat(70_000)}` }],
  ])('gets a fresh 402 for %s, without looking a payment up', async (_, headers) => {
    const api = fakeMitu()

    const response = refused(await testMitu(api).admit(get(headers), price))

    expect(response.status).toBe(402)
    expect(api.calls.map((call) => call.method)).toEqual(['POST', 'GET'])
    expect(api.agentCalls()[0]?.headers.has('authorization')).toBe(false)
  })
})

describe('a credential Mitu refuses', () => {
  it.each([
    [403, 'replay', {}],
    [409, 'already-paid', {}],
    [410, 'expired', {}],
    [402, 'verification-failed', { 'www-authenticate': 'Payment id="fresh"', 'payment-required': 'e30=' }],
    [503, 'settlement-unavailable', { 'retry-after': '20' }],
  ])('passes Mitu’s %i through unchanged and is not admitted', async (status, slug, headers) => {
    const api = fakeMitu({ settle: () => problem(status, slug, headers) })
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    const response = refused(await mitu.admit(get({ authorization: credential }), price))

    expect(response.status).toBe(status)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(response.headers.get('content-type')).toBe('application/problem+json')
    for (const [name, value] of Object.entries(headers)) expect(response.headers.get(name)).toBe(value)
    expect(await response.json()).toMatchObject({ type: `https://paymentauth.org/problems/${slug}` })
  })
})

describe('when Mitu is down', () => {
  it.each([['unreachable' as const], [500], [502], [503]])('answers 503 with Retry-After (API %s)', async (apiDown) => {
    const api = fakeMitu({ apiDown })

    const response = refused(await testMitu(api).admit(get(), price))

    expect(response.status).toBe(503)
    expect(response.headers.get('retry-after')).toBe('5')
    expect(response.headers.get('cache-control')).toBe('no-store')
  })

  it('answers 503 when the payment lookup fails', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))
    api.fetch.mockRejectedValueOnce(new TypeError('fetch failed'))

    expect(refused(await mitu.admit(get({ authorization: credential }), price)).status).toBe(503)
  })

  it('answers 503 when agent_url is unreachable', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))
    api.failAgentUrl('unreachable')

    expect(refused(await mitu.admit(get({ authorization: credential }), price)).status).toBe(503)
  })

  it('answers 503 when agent_url fails to issue a challenge', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)
    api.failAgentUrl(502)

    expect(refused(await mitu.admit(get(), price)).status).toBe(503)
  })

  it.each([302, 307])('answers 503 when agent_url redirects (%i) a credential', async (status) => {
    const api = fakeMitu({ settle: () => new Response(null, { status, headers: { location: 'https://elsewhere.test' } }) })
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    expect(refused(await mitu.admit(get({ authorization: credential }), price)).status).toBe(503)
  })

  it('answers 503 when Mitu says 200 without settling the payment', async () => {
    const api = fakeMitu({ settle: () => Response.json({ status: 'pending' }) })
    const mitu = testMitu(api)
    const credential = mppCredential(await firstChallenge(mitu))

    expect(refused(await mitu.admit(get({ authorization: credential }), price)).status).toBe(503)
  })

  it('answers 503 when Mitu returns a payment it cannot read', async () => {
    const api = fakeMitu()
    api.fetch.mockResolvedValueOnce(Response.json({ data: { id: 'pmt_1', agent_url: 'http://agent.mitu.test/pay/pmt_1' } }))

    expect(refused(await testMitu(api).admit(get(), price)).status).toBe(503)
  })

  it('throws a Mitu API error that retrying cannot fix', async () => {
    const api = fakeMitu({ apiDown: 401 })

    await expect(testMitu(api).admit(get(), price)).rejects.toMatchObject({ status: 401, retryable: false })
    await expect(testMitu(api).admit(get(), price)).rejects.toBeInstanceOf(MituApiError)
  })
})

describe('a free route', () => {
  it.each([
    ['a null price', null],
    ['a price function returning null', () => null],
  ])('admits the request with %s and never calls Mitu', async (_, free) => {
    const api = fakeMitu()

    const admission = await testMitu(api).admit(get({ authorization: 'Payment abc' }), free)

    expect(admission).toEqual({ admitted: true, payment: null, headers: new Headers() })
    expect(api.fetch).not.toHaveBeenCalled()
  })

  it('prices per request with a function of the request', async () => {
    const api = fakeMitu()
    const mitu = testMitu(api)

    await mitu.admit(get({ 'x-tier': 'pro' }), (request) => ({ amount: request.headers.get('x-tier') === 'pro' ? '1.00' : '0.10' }))

    expect(api.calls[0]?.body).toMatchObject({ amount: 1, currency: 'USD' })
  })
})

describe('secrets', () => {
  it('never sends the api key to agent_url, and never logs keys, credentials, receipts or bodies', async () => {
    const api = fakeMitu()
    const { logger, lines } = recordingLogger()
    const mitu = testMitu(api, { logger })
    const challenge = await firstChallenge(mitu)
    const mpp = mppCredential(challenge)
    const x402 = await x402Credential(challenge)
    await mitu.admit(get({ authorization: mpp }), price)
    await mitu.admit(get({ 'payment-signature': x402 }), price)
    await mitu.admit(get({ authorization: 'Payment garbage' }), price)

    for (const call of api.agentCalls()) expect([...call.headers.values()].join()).not.toContain(API_KEY)
    const logged = lines.join('\n')
    expect(lines.length).toBeGreaterThan(0)
    for (const secret of [API_KEY, mpp.slice(8, 40), x402.slice(0, 40), MPP_RECEIPT, X402_RECEIPT, TX_HASH, 'secret problem detail']) {
      expect(logged).not.toContain(secret)
    }
  })

  it('never puts the api key in an error', async () => {
    const api = fakeMitu({ apiDown: 400 })

    const error: unknown = await testMitu(api).admit(get(), price).catch((e: unknown) => e)

    expect(String(error)).not.toContain(API_KEY)
  })
})

describe('createMitu', () => {
  it.each([
    [{ apiKey: 'pk_live_0123456789abcdef' }],
    [{ apiKey: API_KEY, baseUrl: 'http://api.mitu.ai' }],
    [{ apiKey: API_KEY, defaultCurrency: 'dollars!' }],
  ])('refuses invalid options %#', (options) => {
    expect(() => createMitu(options)).toThrow(MituConfigError)
  })

  it('calls the Mitu API at api.mitu.ai/economy by default', async () => {
    const api = fakeMitu()
    const mitu = createMitu({ apiKey: API_KEY, fetch: api.fetch })

    await mitu.admit(get(), price).catch(() => undefined)

    expect(api.calls[0]?.url.href).toBe('https://api.mitu.ai/economy/v1/public/payments')
  })

  it('refuses an invalid price when a request is gated', async () => {
    const mitu = testMitu(fakeMitu())

    await expect(mitu.admit(get(), { amount: '0.001' })).rejects.toThrow(MituConfigError)
  })
})
