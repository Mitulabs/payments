export type MituErrorCode = 'MITU_CONFIG' | 'MITU_API'

/** Base class for every error the package throws. Messages never contain keys, credentials or request data. */
export class MituError extends Error {
  readonly code: MituErrorCode

  constructor(code: MituErrorCode, message: string, options?: ErrorOptions) {
    super(message, options)
    this.name = new.target.name
    this.code = code
  }
}

export class MituConfigError extends MituError {
  constructor(message: string, options?: ErrorOptions) {
    super('MITU_CONFIG', message, options)
  }
}

export interface MituApiErrorOptions extends ErrorOptions {
  /** Overrides the retryability the status implies. */
  retryable?: boolean
}

/** A failed call to Mitu. `status` is 0 when no status is known, as when the request never got a response. */
export class MituApiError extends MituError {
  readonly status: number
  /** Network failures, rate limiting and server errors are worth retrying; redirects and other 4xx are not. */
  readonly retryable: boolean

  constructor(status: number, message: string, { retryable, ...options }: MituApiErrorOptions = {}) {
    super('MITU_API', message, options)
    this.status = status
    this.retryable = retryable ?? (status === 0 || status === 429 || status >= 500)
  }
}
