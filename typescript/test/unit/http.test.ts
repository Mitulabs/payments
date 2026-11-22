import { describe, expect, it, vi } from 'vitest'
import { MituApiError } from '../../src/core/errors.js'
import { createHttpClient } from '../../src/core/http.js'

const API_KEY = 'sk_test_abcdefghijklmnop'

function clientReturning(response: Response | Error) {
  const fetch = vi.fn<typeof globalThis.fetch>(async () => {
    if (response instanceof Error) throw response
    return response
  })
  return { fetch, client: createHttpClient({ apiKey: API_KEY, baseUrl: 'https://mitu.test/economy', fetch }) }
}

describe('createHttpClient', () => {
  it('authenticates, sends JSON, and does not follow redirects', async () => {
    const { fetch, client } = clientReturning(Response.json({ status: 'received' }, { status: 202 }))

    const result = await client.request({ method: 'POST', path: '/v1/x', body: { a: 1 } })

    expect(result).toEqual({ status: 'received' })
    const [url, init] = fetch.mock.calls[0] ?? []
    expect(url).toBe('https://mitu.test/economy/v1/x')
    expect(init?.redirect).toBe('manual')
    expect(init?.body).toBe('{"a":1}')
    expect(init?.headers).toMatchObject({
      'x-api-key': API_KEY,
      'content-type': 'application/json',
    })
    expect(init?.signal).toBeInstanceOf(AbortSignal)
  })

  it('reports the status and error code without the key or body', async () => {
    const body = { error: 'no_receiving_address', detail: 'secret detail' }
    const { client } = clientReturning(Response.json(body, { status: 409 }))

    const error = await client.request({ method: 'GET', path: '/v1/x' }).catch((e: unknown) => e)

    expect(error).toBeInstanceOf(MituApiError)
    expect(error).toMatchObject({ status: 409, retryable: false })
    expect(String(error)).toContain('no_receiving_address')
    expect(String(error)).not.toContain('secret detail')
    expect(String(error)).not.toContain(API_KEY)
  })

  it('turns network failures into a retryable error', async () => {
    const { client } = clientReturning(new TypeError('fetch failed'))

    const error = await client.request({ method: 'GET', path: '/v1/x' }).catch((e: unknown) => e)

    expect(error).toMatchObject({ status: 0, retryable: true })
  })

  it.each([301, 302, 307, 308])('turns a %i redirect into a non-retryable error that names the status', async (status) => {
    const { client } = clientReturning(new Response(null, { status, headers: { location: 'https://elsewhere.test/steal' } }))

    const error = await client.request({ method: 'GET', path: '/v1/x' }).catch((e: unknown) => e)

    expect(error).toBeInstanceOf(MituApiError)
    expect(error).toMatchObject({ status, retryable: false })
    expect(String(error)).toContain(`redirected (${status})`)
  })

  it('treats an opaque redirect as non-retryable', async () => {
    const opaque = new Response(null)
    Object.defineProperty(opaque, 'type', { value: 'opaqueredirect' })
    Object.defineProperty(opaque, 'status', { value: 0 })
    const { client } = clientReturning(opaque)

    await expect(client.request({ method: 'GET', path: '/v1/x' })).rejects.toMatchObject({ status: 0, retryable: false })
  })

  it.each([
    [429, true],
    [500, true],
    [503, true],
    [400, false],
    [401, false],
    [404, false],
  ])('marks status %i retryable=%s', async (status, retryable) => {
    const { client } = clientReturning(new Response('oops', { status }))
    await expect(client.request({ method: 'GET', path: '/v1/x' })).rejects.toMatchObject({ status, retryable })
  })
})
