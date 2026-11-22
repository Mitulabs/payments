/**
 * The package logs only through a logger the app passes in. Nothing it logs
 * contains an API key, a payment credential, a receipt or a request or response body.
 */
export interface Logger {
  debug(message: string, fields?: Record<string, unknown>): void
  info(message: string, fields?: Record<string, unknown>): void
  warn(message: string, fields?: Record<string, unknown>): void
  error(message: string, fields?: Record<string, unknown>): void
}

const noop = (): void => {}

export const silentLogger: Logger = { debug: noop, info: noop, warn: noop, error: noop }
