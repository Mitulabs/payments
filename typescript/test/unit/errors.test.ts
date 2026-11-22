import { describe, expect, it } from 'vitest'
import { MituApiError, MituConfigError } from '../../src/core/errors.js'

describe('MituConfigError', () => {
  it('keeps its own name and code', () => {
    expect(new MituConfigError('bad key')).toMatchObject({ name: 'MituConfigError', code: 'MITU_CONFIG' })
  })
})

describe('MituApiError', () => {
  it.each([
    [0, true],
    [429, true],
    [503, true],
    [404, false],
  ])('treats status %i as retryable=%s', (status, retryable) => {
    expect(new MituApiError(status, 'failed')).toMatchObject({ code: 'MITU_API', status, retryable })
  })

  it('lets the caller override retryability', () => {
    expect(new MituApiError(302, 'redirected', { retryable: false }).retryable).toBe(false)
    expect(new MituApiError(200, 'unreadable', { retryable: true }).retryable).toBe(true)
  })
})
