package mitu

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	paymentTTL = 15 * time.Minute
	retryAfter = 5 * time.Second
)

var (
	// receiptHeaders are Mitu's settlement receipts: MPP's, then x402's v2
	// and v1 spellings. They are added to the paid route's response.
	receiptHeaders = []string{"Payment-Receipt", "Payment-Response", "X-Payment-Response"}

	// relayedHeaders are copied from a Mitu response the agent gets as is.
	relayedHeaders = []string{"Content-Type", "Www-Authenticate", "Payment-Required", "Retry-After"}
)

// Price is a route's price in the merchant's currency. Set exactly one of
// Amount and AmountFunc.
type Price struct {
	// Amount is a fixed decimal price greater than 0 with at most 2 decimal
	// places, such as "5000" or "12.50".
	Amount string
	// AmountFunc prices each request. It is called on the unpaid request and
	// again on the paid retry, and must return the same amount both times,
	// such as a stored order total; it must never read the amount from the
	// request body. An empty amount serves the request for free. An error,
	// or an invalid amount, answers 500.
	AmountFunc func(r *http.Request) (string, error)
	// Currency is the price's currency code. It defaults to
	// Options.DefaultCurrency.
	Currency string
	// Description is shown to the paying agent.
	Description string
}

// Payment is a payment Mitu settled for a request.
type Payment struct {
	ID            string // Mitu's payment id
	Reference     string // the reference the payment was created with
	TxHash        string
	AmountPaid    string // decimal, in Currency
	Currency      string
	Network       string
	PaymentMethod string
}

type paymentKey struct{}

// PaymentFromContext returns the payment for a request served through
// Charge. It reports false for a request served free.
func PaymentFromContext(ctx context.Context) (Payment, bool) {
	p, ok := ctx.Value(paymentKey{}).(Payment)
	return p, ok
}

// Charge returns net/http middleware, also usable with chi, that serves a
// request only once Mitu has settled a payment for it at p.
//
// An unpaid request gets a new Mitu payment for this method, path and price,
// and Mitu's 402 with its MPP and x402 challenges. A paid retry is served
// only when the payment its credential answers was made for this route at
// this price; Mitu then settles it, and the handler runs with the payment in
// its context and Mitu's receipt headers on its response. A payment Mitu
// refuses gets Mitu's answer, and the handler does not run. When Mitu is
// unreachable the route answers 503 with Retry-After.
//
// Charge panics if p is invalid, so a bad price fails at route setup.
func (m *Mitu) Charge(p Price) func(http.Handler) http.Handler {
	if (p.Amount == "") == (p.AmountFunc == nil) {
		panic(fmt.Errorf("%w: a Price needs exactly one of Amount and AmountFunc", ErrConfig))
	}
	currency, err := normalizeCurrency(cmp.Or(p.Currency, m.defaultCurrency))
	if err != nil {
		panic(fmt.Errorf("%w: Price.Currency %w", ErrConfig, err))
	}
	if p.AmountFunc == nil {
		if _, err := parsePrice(p.Amount); err != nil {
			panic(fmt.Errorf("%w: Price.Amount %w", ErrConfig, err))
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			amount, err := p.amount(r)
			if err != nil {
				m.log.Error("mitu: price unavailable", "error", err.Error())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if amount.units == nil {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			t := terms{amount: amount, currency: currency, description: p.Description}
			if cred, ok := readCredential(r); ok && m.pay(w, r, next, t, cred) {
				return
			}
			m.challenge(w, r, t)
		})
	}
}

// amount returns the request's price, or a zero decimal for a free one.
func (p Price) amount(r *http.Request) (decimal, error) {
	s := p.Amount
	if p.AmountFunc != nil {
		var err error
		if s, err = p.AmountFunc(r); err != nil || s == "" {
			return decimal{}, err
		}
	}
	return parsePrice(s)
}

type terms struct {
	amount      decimal
	currency    string
	description string
}

// pay serves a request carrying a credential and reports true, or reports
// false, having written nothing, when the credential does not answer a
// payment for this route at these terms and so gets a fresh 402.
func (m *Mitu) pay(w http.ResponseWriter, r *http.Request, next http.Handler, t terms, cred credential) bool {
	if cred.paymentID == "" {
		return false
	}
	p, err := m.client.getPayment(r.Context(), cred.paymentID)
	if err != nil {
		if e, ok := errors.AsType[*apiError](err); ok && e.status == http.StatusNotFound {
			return false
		}
		m.failed(w, err)
		return true
	}
	if !t.match(p, r) {
		return false
	}

	resp, err := m.client.agent(r.Context(), p.AgentURL, cred.forward)
	if err != nil {
		m.failed(w, err)
		return true
	}
	if resp.status != http.StatusOK {
		relay(w, resp)
		return true
	}

	for _, name := range receiptHeaders {
		if v := resp.header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	ctx := context.WithValue(r.Context(), paymentKey{}, settled(p, resp.body))
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// match reports whether p was made for this route at these terms, so a
// cheaper payment made for another route never unlocks this one.
func (t terms) match(p payment, r *http.Request) bool {
	amount, ok := parseDecimal(p.Amount)
	return ok && amount.cmp(t.amount) == 0 &&
		strings.EqualFold(p.Currency, t.currency) &&
		p.Metadata["method"] == r.Method &&
		p.Metadata["path"] == r.URL.Path
}

// settled reads the payment from Mitu's paid response. Mitu has settled it
// whatever the body says, so an unreadable body still gives the payment.
func settled(p payment, body []byte) Payment {
	var paid struct {
		Receipt struct {
			TxHash        string `json:"tx_hash"`
			AmountPaid    string `json:"amount_paid"`
			Currency      string `json:"currency"`
			Network       string `json:"network"`
			PaymentMethod string `json:"payment_method"`
		} `json:"receipt"`
	}
	_ = json.Unmarshal(body, &paid)
	return Payment{
		ID:            p.ID,
		Reference:     p.Reference,
		TxHash:        paid.Receipt.TxHash,
		AmountPaid:    paid.Receipt.AmountPaid,
		Currency:      paid.Receipt.Currency,
		Network:       paid.Receipt.Network,
		PaymentMethod: paid.Receipt.PaymentMethod,
	}
}

// challenge creates a payment for this request and relays Mitu's 402.
func (m *Mitu) challenge(w http.ResponseWriter, r *http.Request, t terms) {
	p, err := m.client.createPayment(r.Context(), createPayment{
		Amount:      json.Number(t.amount.String()),
		Currency:    t.currency,
		Reference:   "sdk_" + rand.Text(),
		Description: t.description,
		ExpiresAt:   m.now().Add(paymentTTL).UTC().Format(time.RFC3339),
		Metadata:    map[string]string{"sdk": sdkName, "method": r.Method, "path": r.URL.Path},
	})
	if err != nil {
		m.failed(w, err)
		return
	}
	resp, err := m.client.agent(r.Context(), p.AgentURL, http.Header{})
	if err != nil {
		m.failed(w, err)
		return
	}
	if resp.status >= http.StatusInternalServerError {
		m.failed(w, &apiError{status: resp.status, message: fmt.Sprintf("GET agent_url failed (%d)", resp.status)})
		return
	}
	relay(w, resp)
}

// relay writes Mitu's answer to the agent: its status, protocol headers and
// body, and nothing else.
func relay(w http.ResponseWriter, resp *response) {
	for _, name := range relayedHeaders {
		if v := resp.header.Values(name); len(v) > 0 {
			w.Header()[name] = v
		}
	}
	w.WriteHeader(resp.status)
	_, _ = w.Write(resp.body)
}

// failed answers a request Mitu could not serve: 503 with Retry-After when
// Mitu is unavailable, 500 when it refused this package's call.
func (m *Mitu) failed(w http.ResponseWriter, err error) {
	if e, ok := errors.AsType[*apiError](err); ok && !e.unavailable() {
		m.log.Error("mitu: request refused", "error", err.Error())
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	m.log.Warn("mitu: unavailable", "error", err.Error())
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
}
