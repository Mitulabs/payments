import { describe, expect, it } from 'vitest'
import { assertApiKey, resolveBaseUrl } from '../../src/core/environment.js'
import { MituConfigError } from '../../src/core/errors.js'

describe('assertApiKey', () => {
  it.each(['sk_test_0123456789abcdef0123456789abcdef', `sk_live_${'0123456789abcdef'.repeat(2)}`])('accepts %s', (key) => {
    expect(() => {
      assertApiKey(key)
    }).not.toThrow()
  })

  it.each(['', 'sk_test_', 'sk_live_short', 'sk_test_abc-defghijklmnop', 'pk_live_Zq81Vx0Lm2Pp7Rr4', 'sk_prod_Zq81Vx0Lm2Pp7Rr4'])(
    'rejects the key %j',
    (key) => {
      expect(() => {
        assertApiKey(key)
      }).toThrow(MituConfigError)
    },
  )

  it('never echoes a rejected key', () => {
    const error = captureError(() => {
      assertApiKey('pk_live_Zq81Vx0Lm2Pp7Rr4')
    })
    expect(String(error)).not.toContain('Zq81Vx0Lm2Pp7Rr4')
  })
})

describe('resolveBaseUrl', () => {
  it('defaults to the Mitu API behind the gateway', () => {
    expect(resolveBaseUrl()).toBe('https://api.mitu.ai/economy')
  })

  it('keeps the path and drops a trailing slash, query and fragment', () => {
    expect(resolveBaseUrl('https://mitu.test/economy/?x=1#y')).toBe('https://mitu.test/economy')
    expect(resolveBaseUrl('https://mitu.test')).toBe('https://mitu.test')
  })

  it('accepts plain http to this machine', () => {
    expect(resolveBaseUrl('http://localhost:4003')).toBe('http://localhost:4003')
    expect(resolveBaseUrl('http://127.0.0.1:4003')).toBe('http://127.0.0.1:4003')
    expect(resolveBaseUrl('http://[::1]:4003')).toBe('http://[::1]:4003')
  })

  it.each(['http://api.mitu.ai', 'http://localhost.evil.com', 'ftp://api.mitu.ai', 'not a url'])('refuses %s', (url) => {
    expect(() => resolveBaseUrl(url)).toThrow(MituConfigError)
  })
})

function captureError(fn: () => void): unknown {
  try {
    fn()
  } catch (error) {
    return error
  }
  throw new Error('expected an error')
}
