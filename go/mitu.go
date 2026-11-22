// Package mitu puts Mitu's payment flow in front of Go HTTP routes, so AI
// agents pay for them over the Machine Payments Protocol (MPP) or x402.
//
// Mitu issues the payment challenges, settles each payment on chain, locks
// the exchange rate when the payment is created and notifies the merchant
// with its payment.completed webhook. This package only creates the payment
// for a request, relays Mitu's challenge to the agent, and serves the route
// once Mitu has settled the agent's payment.
package mitu

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{2,10}$`)

// Options configures New.
type Options struct {
	// APIKey is the Mitu secret key. sk_test_ keys use the sandbox and
	// sk_live_ keys production. It is sent only to the Mitu API.
	APIKey string
	// DefaultCurrency is the currency of a Price that names none, such as
	// "USD" or "NGN". It defaults to "USD".
	DefaultCurrency string
	// BaseURL overrides the Mitu API origin. It must use https unless it
	// points at this machine.
	BaseURL string
	// HTTPClient makes the calls to Mitu. It defaults to a new http.Client.
	// Redirects are never followed, and every call is bounded by a timeout.
	HTTPClient *http.Client
	// Logger receives the package's logs, which never contain keys,
	// credentials, receipts or bodies. It defaults to discarding them.
	Logger *slog.Logger
}

// Mitu charges agents for HTTP routes through Mitu. Create it with New. It
// is safe for concurrent use and holds no background resources.
type Mitu struct {
	client          *client
	log             *slog.Logger
	defaultCurrency string
	now             func() time.Time
}

// New validates opts and returns a Mitu. It makes no network call, so it
// succeeds while Mitu is unreachable. Invalid options give an error wrapping
// ErrConfig; a done ctx gives ctx.Err().
func New(ctx context.Context, opts Options) (*Mitu, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, baseURL, err := resolveEnvironment(opts.APIKey, opts.BaseURL)
	if err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(cmp.Or(opts.DefaultCurrency, "USD"))
	if err != nil {
		return nil, fmt.Errorf("%w: DefaultCurrency %w", ErrConfig, err)
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Mitu{
		client:          newClient(opts.APIKey, baseURL, opts.HTTPClient),
		log:             log,
		defaultCurrency: currency,
		now:             time.Now,
	}, nil
}

func normalizeCurrency(c string) (string, error) {
	c = strings.ToUpper(c)
	if !currencyPattern.MatchString(c) {
		return "", errInvalidCurrency
	}
	return c, nil
}
