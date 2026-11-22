/** A payment Mitu settled for this request. */
export interface Payment {
  /** Mitu's payment id, as in the `payment.completed` webhook. */
  id: string
  /** The reference the SDK gave the payment, unique per merchant. */
  reference: string
  /** The settlement transaction, or `null` where there is none on chain. */
  txHash: string | null
  /** What the agent paid, in `currency`. */
  amountPaid: string
  currency: string
  /** The network it settled on, such as `base`, `tempo` or `solana`. */
  network: string | null
  /** How it was paid, as Mitu names the method. */
  paymentMethod: string | null
}

export interface PaymentContext {
  /** The settled payment, or `null` when the route was free for this request. */
  payment: Payment | null
}

/**
 * The outcome of gating one request. Admitted: serve it, then add `headers`
 * (the payment receipt) to the response. Otherwise send `response` as it is:
 * a payment challenge, a refused credential or a 503.
 */
export type Admission =
  | { admitted: true; payment: Payment | null; headers: Headers }
  | { admitted: false; response: Response }
