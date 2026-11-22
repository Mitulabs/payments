import { MituConfigError } from './errors.js'

const API_KEY = /^sk_(test|live)_[A-Za-z0-9]{16,}$/

/** The Mitu API for each kind of key. */
const BASE_URLS = {
  live: 'https://api.mitu.ai',
  test: 'https://sandbox.mitu.ai',
} as const

export function assertApiKey(apiKey: string): void {
  if (!API_KEY.test(apiKey)) {
    throw new MituConfigError('apiKey must be a Mitu secret key (sk_test_... or sk_live_...)')
  }
}

/** The Mitu API for the key: production for `sk_live_` keys, the sandbox for `sk_test_` keys. */
export const apiUrlFor = (apiKey: string): string => BASE_URLS[apiKey.startsWith('sk_live_') ? 'live' : 'test']
