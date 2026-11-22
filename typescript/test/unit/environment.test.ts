import { describe, expect, it } from 'vitest'
import { apiUrlFor, assertApiKey } from '../../src/core/environment.js'
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

describe('apiUrlFor', () => {
  it('sends live keys to production and test keys to the sandbox', () => {
    expect(apiUrlFor(`sk_live_${'0123456789abcdef'.repeat(2)}`)).toBe('https://api.mitu.ai')
    expect(apiUrlFor('sk_test_0123456789abcdef0123456789abcdef')).toBe('https://sandbox.mitu.ai')
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
