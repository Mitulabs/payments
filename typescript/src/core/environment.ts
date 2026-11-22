import { MituConfigError } from './errors.js'

const API_KEY = /^sk_(test|live)_[A-Za-z0-9]{16,}$/

/** The Mitu API, shared by live and test keys. */
export const DEFAULT_BASE_URL = 'https://api.mitu.ai/economy'

export function assertApiKey(apiKey: string): void {
  if (!API_KEY.test(apiKey)) {
    throw new MituConfigError('apiKey must be a Mitu secret key (sk_test_... or sk_live_...)')
  }
}

/** HTTPS, or plain HTTP to this machine. */
export function isAllowedUrl(url: URL): boolean {
  const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]'
  return url.protocol === 'https:' || (local && url.protocol === 'http:')
}

/** The API base URL without a trailing slash. An explicit one must be HTTPS unless it points at this machine. */
export function resolveBaseUrl(baseUrl = DEFAULT_BASE_URL): string {
  const url = URL.parse(baseUrl)
  if (!url) throw new MituConfigError('baseUrl is not a valid URL')
  if (!isAllowedUrl(url)) throw new MituConfigError('baseUrl must use https')
  return url.origin + url.pathname.replace(/\/+$/, '')
}
