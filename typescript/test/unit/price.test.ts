import { describe, expect, it } from 'vitest'
import { MituConfigError } from '../../src/core/errors.js'
import { assertPrice, checkPrice, resolvePrice, sameAmount } from '../../src/core/price.js'

describe('checkPrice', () => {
  it.each(['1', '0.01', '0.5', '2500', '2500.00'])('accepts %s', (amount) => {
    expect(checkPrice({ amount }, 'USD')).toEqual({ amount, currency: 'USD' })
  })

  it.each(['0', '0.00', '-1', '0.001', '1.', '.5', '1e3', ' 1', 'NaN', ''])('refuses %j', (amount) => {
    expect(() => checkPrice({ amount }, 'USD')).toThrow(MituConfigError)
  })

  it('upper-cases the currency and keeps the description', () => {
    expect(checkPrice({ amount: '5', currency: 'ngn', description: 'Report' }, 'USD')).toEqual({
      amount: '5',
      currency: 'NGN',
      description: 'Report',
    })
  })

  it.each(['U', 'US$', 'ÜSD'])('refuses the currency %j', (currency) => {
    expect(() => checkPrice({ amount: '5', currency }, 'USD')).toThrow(MituConfigError)
  })
})

describe('assertPrice', () => {
  it('checks fixed prices and leaves null and functions to request time', () => {
    expect(() => {
      assertPrice({ amount: '0' }, 'USD')
    }).toThrow(MituConfigError)
    expect(() => {
      assertPrice(null, 'USD')
      assertPrice(() => ({ amount: '0' }), 'USD')
    }).not.toThrow()
  })
})

describe('resolvePrice', () => {
  it('resolves a function of the input, sync or async', async () => {
    expect(await resolvePrice((n: number) => ({ amount: String(n) }), 3, 'NGN')).toEqual({ amount: '3', currency: 'NGN' })
    expect(await resolvePrice(async () => null, 0, 'USD')).toBeNull()
  })
})

describe('sameAmount', () => {
  it.each([
    ['5', '5.00', true],
    ['0.5', '0.50', true],
    ['0.05', '0.50', false],
    ['5', 'five', false],
  ])('%s and %s: %s', (a, b, same) => {
    expect(sameAmount(a, b)).toBe(same)
  })
})
