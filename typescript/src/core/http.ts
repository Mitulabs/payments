import { MituApiError } from './errors.js'
import { isRecord } from './guards.js'

export const PACKAGE_NAME = '@mitulabs/payments'
export const PACKAGE_VERSION = '0.1.1'
export const USER_AGENT = `mitulabs-payments-js/${PACKAGE_VERSION}`

export const TIMEOUT_MS = 10_000

export interface HttpRequest {
  method: 'GET' | 'POST'
  path: `/${string}`
  body?: object
}

export interface HttpClient {
  request(request: HttpRequest): Promise<unknown>
}

export interface HttpClientOptions {
  apiKey: string
  baseUrl: string
  fetch: typeof fetch
}

/**
 * Talks to the Mitu API. Every request is authenticated with `x-api-key`,
 * bounded by a timeout and never follows a redirect, which would carry the key
 * to whatever host it names. Errors carry the status and Mitu's error code only.
 */
export function createHttpClient({ apiKey, baseUrl, fetch }: HttpClientOptions): HttpClient {
  return {
    async request({ method, path, body }) {
      const init: RequestInit = {
        method,
        redirect: 'manual',
        signal: AbortSignal.timeout(TIMEOUT_MS),
        headers: {
          accept: 'application/json',
          'user-agent': USER_AGENT,
          'x-api-key': apiKey,
          ...(body && { 'content-type': 'application/json' }),
        },
        ...(body && { body: JSON.stringify(body) }),
      }

      let response: Response
      try {
        response = await fetch(baseUrl + path, init)
      } catch (error) {
        throw new MituApiError(0, `Mitu API ${method} ${path} unreachable`, { cause: error })
      }
      if (isRedirect(response)) {
        await response.body?.cancel()
        const status = response.status ? ` (${response.status})` : ''
        throw new MituApiError(response.status, `Mitu API ${method} ${path} redirected${status}`, { retryable: false })
      }
      const payload = await readJson(response)

      if (!response.ok) {
        throw new MituApiError(response.status, `Mitu API ${method} ${path} failed (${response.status}${errorCode(payload)})`)
      }
      return payload
    },
  }
}

/** Runtimes that hide a manual redirect return an opaque response with status 0. */
export function isRedirect(response: Response): boolean {
  return response.type === 'opaqueredirect' || (response.status >= 300 && response.status < 400)
}

export async function readJson(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return undefined
  try {
    return JSON.parse(text) as unknown
  } catch {
    return undefined
  }
}

function errorCode(payload: unknown): string {
  if (isRecord(payload) && typeof payload.error === 'string') {
    return `: ${payload.error.slice(0, 64)}`
  }
  return ''
}
