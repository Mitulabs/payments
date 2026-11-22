import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { PACKAGE_VERSION } from '../../src/core/http.js'

describe('PACKAGE_VERSION', () => {
  it('matches package.json', () => {
    const manifest: unknown = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'))
    expect(manifest).toMatchObject({ version: PACKAGE_VERSION })
  })
})
