package mitu_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mitu "github.com/mitulabs/payments/go"
	"github.com/mitulabs/payments/go/internal/mitutest"
)

// route is a charged handler that records whether it ran and the payment
// it was served with.
type route struct {
	http.Handler
	ran     atomic.Int32
	payment atomic.Pointer[mitu.Payment]
}

func newRoute(m *mitu.Mitu, p mitu.Price) *route {
	rt := &route{}
	rt.Handler = m.Charge(p)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt.ran.Add(1)
		if pay, ok := mitu.PaymentFromContext(r.Context()); ok {
			rt.payment.Store(&pay)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "served")
	}))
	return rt
}

// newFake starts a fake Mitu and, when the test ends, checks that the API
// key never reached agent_url and that nothing logged a secret.
func newFake(t *testing.T) (*mitutest.Fake, *mitu.Mitu) {
	t.Helper()
	f := mitutest.New(t)
	logs := &syncBuffer{}
	m := f.Mitu(t, mitu.Options{Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	t.Cleanup(func() {
		for _, c := range f.Calls() {
			if !c.Agent {
				continue
			}
			for name, values := range c.Header {
				for _, v := range values {
					if strings.Contains(v, mitutest.APIKey) {
						t.Errorf("API key sent to agent_url in %s", name)
					}
				}
			}
		}
		for _, secret := range []string{mitutest.APIKey, "Payment ey", "bXBwLXJlY2VpcHQ", "eDQwMi1yZWNlaXB0", "0xsigned"} {
			if strings.Contains(logs.String(), secret) {
				t.Errorf("logs contain %q:\n%s", secret, logs.String())
			}
		}
	})
	return f, m
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func serve(h http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func wantStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d; body %s", w.Code, status, w.Body)
	}
}

// agentCalls returns the calls made to agent_url.
func agentCalls(f *mitutest.Fake) []mitutest.Call {
	var calls []mitutest.Call
	for _, c := range f.Calls() {
		if c.Agent {
			calls = append(calls, c)
		}
	}
	return calls
}

func TestUnpaidRequestCreatesPaymentAndRelaysMitu402(t *testing.T) {
	t.Parallel()
	f, m := newFake(t)
	rt := newRoute(m, mitu.Price{Amount: "0.5", Currency: "usd", Description: "Search"})

	before := time.Now()
	w := serve(rt, http.MethodGet, "/v1/search?q=jollof", nil)

	wantStatus(t, w, http.StatusPaymentRequired)
	if rt.ran.Load() != 0 {
		t.Fatal("handler ran for an unpaid request")
	}
	calls := f.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want create then agent_url", len(calls))
	}

	create := calls[0]
	if create.Agent || create.Method != http.MethodPost || create.Path != "/v1/public/payments" {
		t.Fatalf("first call = %s %s", create.Method, create.Path)
	}
	if got := create.Header.Get("X-Api-Key"); got != mitutest.APIKey {
		t.Errorf("x-api-key = %q", got)
	}
	if !bytes.Contains(create.Body, []byte(`"amount":0.5,`)) {
		t.Errorf("amount not sent as a JSON number: %s", create.Body)
	}
	var body struct {
		Currency    string            `json:"currency"`
		Reference   string            `json:"reference"`
		Description string            `json:"description"`
		ExpiresAt   time.Time         `json:"expires_at"`
		Metadata    map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(create.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Currency != "USD" || body.Description != "Search" {
		t.Errorf("currency, description = %q, %q", body.Currency, body.Description)
	}
	if !regexp.MustCompile(`^sdk_[A-Z2-7]{26}$`).MatchString(body.Reference) {
		t.Errorf("reference = %q, want sdk_ and 128 random bits", body.Reference)
	}
	if lo, hi := before.Add(15*time.Minute-time.Second), time.Now().Add(15*time.Minute+time.Second); body.ExpiresAt.Before(lo) || body.ExpiresAt.After(hi) {
		t.Errorf("expires_at = %v, want 15 minutes from now", body.ExpiresAt)
	}
	wantMeta := map[string]string{"sdk": "github.com/mitulabs/payments/go@" + mitu.Version, "method": "GET", "path": "/v1/search"}
	if len(body.Metadata) != len(wantMeta) {
		t.Errorf("metadata = %v, want %v", body.Metadata, wantMeta)
	}
	for k, v := range wantMeta {
		if body.Metadata[k] != v {
			t.Errorf("metadata[%s] = %q, want %q", k, body.Metadata[k], v)
		}
	}

	fetch := calls[1]
	payment := f.Payments()[0]
	if !fetch.Agent || fetch.Method != http.MethodGet || fetch.Path != "/pay/"+payment.ID {
		t.Fatalf("second call = %s %s on agent %v", fetch.Method, fetch.Path, fetch.Agent)
	}
	for _, h := range []string{"X-Api-Key", "Authorization", "Payment-Signature", "X-Payment", "Cookie"} {
		if fetch.Header.Get(h) != "" {
			t.Errorf("challenge fetch sent %s", h)
		}
	}

	for name, want := range map[string]string{
		"Content-Type":  "application/problem+json",
		"Cache-Control": "no-store",
	} {
		if got := w.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if !strings.Contains(w.Header().Get("Www-Authenticate"), `method="tempo"`) || w.Header().Get("Payment-Required") == "" {
		t.Errorf("challenges not relayed: %v", w.Header())
	}
	for _, h := range []string{"X-Internal", "Set-Cookie"} {
		if w.Header().Get(h) != "" {
			t.Errorf("%s relayed from Mitu", h)
		}
	}
	if !strings.Contains(w.Body.String(), `"resource_id":"`+payment.ID+`"`) {
		t.Errorf("body = %s", w.Body)
	}
}

func TestPaidRequestIsServed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pay      func(t testing.TB, h http.Header) http.Header
		forward  string
		receipts []string
		method   string
		network  string
	}{
		{
			name: "mpp",
			pay: func(t testing.TB, h http.Header) http.Header {
				return http.Header{"Authorization": {mitutest.MPP(t, h)}}
			},
			forward:  "Authorization",
			receipts: []string{"Payment-Receipt"},
			method:   "mpp_crypto",
			network:  "tempo",
		},
		{
			name: "mpp in Payment-Authorization",
			pay: func(t testing.TB, h http.Header) http.Header {
				return http.Header{"Authorization": {"Bearer merchant-session"}, "Payment-Authorization": {mitutest.MPP(t, h)}}
			},
			forward:  "Authorization",
			receipts: []string{"Payment-Receipt"},
			method:   "mpp_crypto",
			network:  "tempo",
		},
		{
			name: "x402",
			pay: func(t testing.TB, h http.Header) http.Header {
				return http.Header{"Payment-Signature": {mitutest.X402(t, h)}}
			},
			forward:  "Payment-Signature",
			receipts: []string{"Payment-Response", "X-Payment-Response"},
			method:   "x402",
			network:  "base",
		},
		{
			name:     "x402 v1 header",
			pay:      func(t testing.TB, h http.Header) http.Header { return http.Header{"X-Payment": {mitutest.X402(t, h)}} },
			forward:  "X-Payment",
			receipts: []string{"Payment-Response", "X-Payment-Response"},
			method:   "x402",
			network:  "base",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			rt := newRoute(m, mitu.Price{Amount: "12.50", Currency: "NGN"})
			unpaid := serve(rt, http.MethodPost, "/orders/42", nil)
			wantStatus(t, unpaid, http.StatusPaymentRequired)

			header := tt.pay(t, unpaid.Header())
			header.Set("Cookie", "session=merchant")
			header.Set("X-Api-Key", "sk_live_agentsuppliedvalue")
			paid := serve(rt, http.MethodPost, "/orders/42?expand=1", header)

			wantStatus(t, paid, http.StatusCreated)
			if paid.Body.String() != "served" || rt.ran.Load() != 1 {
				t.Fatalf("handler ran %d times, body %q", rt.ran.Load(), paid.Body)
			}
			for _, name := range tt.receipts {
				if paid.Header().Get(name) == "" {
					t.Errorf("receipt %s missing: %v", name, paid.Header())
				}
			}
			if paid.Header().Get("X-Internal") != "" {
				t.Error("non-receipt header copied from Mitu's paid response")
			}
			if got := paid.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q", got)
			}

			payment := f.Payments()[0]
			want := mitu.Payment{
				ID: payment.ID, Reference: payment.Reference, TxHash: "0xabc", AmountPaid: "12.50",
				Currency: "NGN", Network: tt.network, PaymentMethod: tt.method,
			}
			if got := rt.payment.Load(); got == nil || *got != want {
				t.Errorf("payment = %+v, want %+v", got, want)
			}

			calls := f.Calls()[2:]
			if len(calls) != 2 {
				t.Fatalf("paid calls = %d, want fetch then settle", len(calls))
			}
			if c := calls[0]; c.Agent || c.Method != http.MethodGet || c.Path != "/v1/public/payments/"+payment.ID || c.Header.Get("X-Api-Key") != mitutest.APIKey {
				t.Errorf("fetch = %s %s on agent %v", c.Method, c.Path, c.Agent)
			}
			settle := calls[1]
			if !settle.Agent || settle.Method != http.MethodGet || settle.Path != "/pay/"+payment.ID {
				t.Fatalf("settle = %s %s on agent %v", settle.Method, settle.Path, settle.Agent)
			}
			if got, want := settle.Header.Get(tt.forward), header.Get(tt.forward); tt.forward != "Authorization" && got != want {
				t.Errorf("forwarded %s = %q, want %q", tt.forward, got, want)
			}
			if !strings.HasPrefix(settle.Header.Get("Authorization"), "Payment ") && tt.forward == "Authorization" {
				t.Errorf("Authorization forwarded = %q", settle.Header.Get("Authorization"))
			}
			for _, h := range []string{"Cookie", "X-Api-Key", "Payment-Authorization"} {
				if settle.Header.Get(h) != "" {
					t.Errorf("%s forwarded to agent_url", h)
				}
			}

			replay := serve(rt, http.MethodPost, "/orders/42", header)
			wantStatus(t, replay, http.StatusConflict)
			if rt.ran.Load() != 1 {
				t.Error("handler ran for a replayed credential")
			}
		})
	}
}

func TestMismatchedPaymentGetsFresh402(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		price        mitu.Price
		method, path string
	}{
		{"another path", mitu.Price{Amount: "1"}, http.MethodGet, "/premium"},
		{"another method", mitu.Price{Amount: "1"}, http.MethodDelete, "/cheap"},
		{"a higher amount", mitu.Price{Amount: "1.01"}, http.MethodGet, "/cheap"},
		{"another currency", mitu.Price{Amount: "1", Currency: "NGN"}, http.MethodGet, "/cheap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			cheap := newRoute(m, mitu.Price{Amount: "1.00"})
			other := newRoute(m, tt.price)
			unpaid := serve(cheap, http.MethodGet, "/cheap", nil)

			w := serve(other, tt.method, tt.path, http.Header{"Authorization": {mitutest.MPP(t, unpaid.Header())}})

			wantStatus(t, w, http.StatusPaymentRequired)
			if other.ran.Load() != 0 {
				t.Fatal("handler ran")
			}
			if n := len(f.Payments()); n != 2 {
				t.Errorf("payments = %d, want a fresh one", n)
			}
			for _, c := range agentCalls(f) {
				if c.Header.Get("Authorization") != "" {
					t.Error("credential forwarded for a mismatched payment")
				}
			}
		})
	}
}

func TestDynamicPriceChangeGetsFresh402(t *testing.T) {
	t.Parallel()
	f, m := newFake(t)
	var amount atomic.Value
	amount.Store("3")
	rt := newRoute(m, mitu.Price{AmountFunc: func(*http.Request) (string, error) { return amount.Load().(string), nil }})
	unpaid := serve(rt, http.MethodGet, "/basket", nil)
	amount.Store("4")

	w := serve(rt, http.MethodGet, "/basket", http.Header{"Payment-Signature": {mitutest.X402(t, unpaid.Header())}})

	wantStatus(t, w, http.StatusPaymentRequired)
	if rt.ran.Load() != 0 || len(f.Payments()) != 2 || f.Payments()[1].Amount != "4.00" {
		t.Fatalf("ran %d, payments %+v", rt.ran.Load(), f.Payments())
	}
}

func TestUnusableCredentialGetsFresh402(t *testing.T) {
	t.Parallel()
	tests := map[string]http.Header{
		"unparseable mpp":    {"Authorization": {"Payment not-a-credential"}},
		"mitu parameters":    {"Authorization": {`Payment id="pmt_1", method="tempo"`}},
		"unparseable x402":   {"Payment-Signature": {"%%%"}},
		"x402 for elsewhere": {"Payment-Signature": {x402For("https://example.com/data")}},
		"unknown payment":    {"Payment-Signature": {x402For("https://api.mitu.ai/pay/pmt_none")}},
	}
	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			rt := newRoute(m, mitu.Price{Amount: "2"})

			w := serve(rt, http.MethodGet, "/data", header)

			wantStatus(t, w, http.StatusPaymentRequired)
			if rt.ran.Load() != 0 || len(f.Payments()) != 1 {
				t.Fatalf("ran %d, payments %d", rt.ran.Load(), len(f.Payments()))
			}
			if w.Header().Get("Www-Authenticate") == "" {
				t.Error("no challenge in the fresh 402")
			}
		})
	}
}

// x402For is an x402 payment payload naming resourceURL.
func x402For(resourceURL string) string {
	b, _ := json.Marshal(map[string]any{"x402Version": 2, "resource": map[string]string{"url": resourceURL}})
	return base64.StdEncoding.EncodeToString(b)
}

func TestMituRefusalIsPassedThrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		reply mitutest.Reply
	}{
		{"replayed credential", mitutest.Reply{
			Status: http.StatusForbidden,
			Header: http.Header{"Content-Type": {"application/problem+json"}},
			Body:   `{"type":"https://paymentauth.org/problems/credential-replayed","status":403}`,
		}},
		{"expired payment", mitutest.Reply{
			Status: http.StatusGone,
			Header: http.Header{"Content-Type": {"application/problem+json"}, "X-Internal": {"secret"}},
			Body:   `{"type":"https://paymentauth.org/problems/payment-expired","status":410}`,
		}},
		{"settlement unavailable", mitutest.Reply{
			Status: http.StatusServiceUnavailable,
			Header: http.Header{"Content-Type": {"application/problem+json"}, "Retry-After": {"20"}},
			Body:   `{"type":"https://paymentauth.org/problems/settlement-unavailable","status":503}`,
		}},
		{"payment rejected", mitutest.Reply{
			Status: http.StatusPaymentRequired,
			Header: http.Header{"Content-Type": {"application/problem+json"}, "Www-Authenticate": {`Payment id="ch_2"`}, "Payment-Required": {"e30="}},
			Body:   `{"type":"https://paymentauth.org/problems/verification-failed","status":402}`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			rt := newRoute(m, mitu.Price{Amount: "2"})
			unpaid := serve(rt, http.MethodGet, "/data", nil)
			f.Settle(func(*mitutest.Payment) *mitutest.Reply { return &tt.reply })

			w := serve(rt, http.MethodGet, "/data", http.Header{"Authorization": {mitutest.MPP(t, unpaid.Header())}})

			wantStatus(t, w, tt.reply.Status)
			if rt.ran.Load() != 0 {
				t.Fatal("handler ran")
			}
			if w.Body.String() != tt.reply.Body {
				t.Errorf("body = %s", w.Body)
			}
			for _, name := range []string{"Content-Type", "Retry-After", "Www-Authenticate", "Payment-Required"} {
				if got, want := w.Header().Get(name), tt.reply.Header.Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
			if w.Header().Get("X-Internal") != "" || w.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("headers = %v", w.Header())
			}
			if len(f.Payments()) != 1 {
				t.Error("refused payment answered with a new payment")
			}
		})
	}
}

func TestMituUnavailableAnswers503(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		paid bool
		fail func(f *mitutest.Fake)
	}{
		{"API down when creating", false, func(f *mitutest.Fake) { f.API.Close() }},
		{"API error when creating", false, func(f *mitutest.Fake) { f.FailAPI(&mitutest.Reply{Status: http.StatusBadGateway}) }},
		{"API rate limited", false, func(f *mitutest.Fake) { f.FailAPI(&mitutest.Reply{Status: http.StatusTooManyRequests}) }},
		{"agent_url down for the challenge", false, func(f *mitutest.Fake) { f.Agent.Close() }},
		{"API error when fetching", true, func(f *mitutest.Fake) { f.FailAPI(&mitutest.Reply{Status: http.StatusInternalServerError}) }},
		{"API down when fetching", true, func(f *mitutest.Fake) { f.API.Close() }},
		{"agent_url down when paying", true, func(f *mitutest.Fake) { f.Agent.Close() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			rt := newRoute(m, mitu.Price{Amount: "2"})
			var header http.Header
			if tt.paid {
				header = http.Header{"Payment-Signature": {mitutest.X402(t, serve(rt, http.MethodGet, "/data", nil).Header())}}
			}
			tt.fail(f)

			w := serve(rt, http.MethodGet, "/data", header)

			wantStatus(t, w, http.StatusServiceUnavailable)
			if got := w.Header().Get("Retry-After"); got != "5" {
				t.Errorf("Retry-After = %q, want 5", got)
			}
			if rt.ran.Load() != 0 {
				t.Fatal("handler ran")
			}
		})
	}
}

func TestMituRefusingTheSDKAnswers500(t *testing.T) {
	t.Parallel()
	tests := map[string]mitutest.Reply{
		"invalid key":  {Status: http.StatusUnauthorized, Body: `{"message":"Invalid API key"}`},
		"redirect":     {Status: http.StatusFound, Header: http.Header{"Location": {"https://example.com/"}}},
		"invalid body": {Status: http.StatusOK, Body: `{"data":{}}`},
		"plain agent_url": {Status: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
			Body: `{"data":{"id":"pmt_1","amount":"2.00","currency":"USD","agent_url":"http://agent.example.com/pay/pmt_1"}}`},
	}
	for name, reply := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			f.FailAPI(&reply)
			rt := newRoute(m, mitu.Price{Amount: "2"})

			w := serve(rt, http.MethodGet, "/data", nil)

			wantStatus(t, w, http.StatusInternalServerError)
			if len(agentCalls(f)) != 0 || rt.ran.Load() != 0 {
				t.Fatal("served or fetched agent_url after Mitu refused")
			}
		})
	}
}

func TestFreeRoute(t *testing.T) {
	t.Parallel()
	f, m := newFake(t)
	rt := newRoute(m, mitu.Price{AmountFunc: func(*http.Request) (string, error) { return "", nil }})
	var paid bool
	free := m.Charge(mitu.Price{AmountFunc: func(*http.Request) (string, error) { return "", nil }})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, paid = mitu.PaymentFromContext(r.Context())
	}))

	wantStatus(t, serve(rt, http.MethodGet, "/sample", nil), http.StatusCreated)
	wantStatus(t, serve(free, http.MethodGet, "/sample", http.Header{"Authorization": {"Payment abc"}}), http.StatusOK)

	if paid || len(f.Calls()) != 0 {
		t.Fatalf("free route: paid %v, calls %d", paid, len(f.Calls()))
	}
}

func TestPriceFailureAnswers500(t *testing.T) {
	t.Parallel()
	for name, fn := range map[string]func(*http.Request) (string, error){
		"error":           func(*http.Request) (string, error) { return "", errors.New("db down") },
		"sub-cent amount": func(*http.Request) (string, error) { return "0.001", nil },
		"zero":            func(*http.Request) (string, error) { return "0", nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, m := newFake(t)
			rt := newRoute(m, mitu.Price{AmountFunc: fn})
			wantStatus(t, serve(rt, http.MethodGet, "/x", nil), http.StatusInternalServerError)
			if len(f.Calls()) != 0 || rt.ran.Load() != 0 {
				t.Fatal("called Mitu or served")
			}
		})
	}
}

func TestChargePanicsOnInvalidPrice(t *testing.T) {
	t.Parallel()
	_, m := newFake(t)
	for name, p := range map[string]mitu.Price{
		"neither":         {},
		"both":            {Amount: "1", AmountFunc: func(*http.Request) (string, error) { return "1", nil }},
		"float notation":  {Amount: "1e2"},
		"three decimals":  {Amount: "1.005"},
		"zero":            {Amount: "0.00"},
		"negative":        {Amount: "-1"},
		"bad currency":    {Amount: "1", Currency: "US$"},
		"single currency": {Amount: "1", Currency: "U"},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				err, _ := recover().(error)
				if !errors.Is(err, mitu.ErrConfig) {
					t.Fatalf("recovered %v, want ErrConfig", err)
				}
			}()
			m.Charge(p)
		})
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string]mitu.Options{
		"missing key":      {},
		"publishable key":  {APIKey: "pk_test_abcdefghijklmnop"},
		"bad currency":     {APIKey: mitutest.APIKey, DefaultCurrency: "N1"},
		"plain http":       {APIKey: mitutest.APIKey, BaseURL: "http://api.mitu.ai"},
		"relative baseURL": {APIKey: mitutest.APIKey, BaseURL: "/v1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mitu.New(t.Context(), opts); !errors.Is(err, mitu.ErrConfig) {
				t.Fatalf("err = %v, want ErrConfig", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mitu.New(ctx, mitu.Options{APIKey: mitutest.APIKey}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := mitu.New(t.Context(), mitu.Options{APIKey: mitutest.APIKey, DefaultCurrency: "ngn"}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultCurrency(t *testing.T) {
	t.Parallel()
	f := mitutest.New(t)
	m := f.Mitu(t, mitu.Options{DefaultCurrency: "ngn"})
	serve(newRoute(m, mitu.Price{Amount: "5000"}), http.MethodGet, "/x", nil)
	if got := f.Payments()[0]; got.Currency != "NGN" || got.Amount != "5000.00" {
		t.Fatalf("payment = %s %s", got.Amount, got.Currency)
	}
}

func TestConcurrentRequests(t *testing.T) {
	t.Parallel()
	f, m := newFake(t)
	rt := newRoute(m, mitu.Price{Amount: "1"})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			unpaid := serve(rt, http.MethodGet, "/c", nil)
			if unpaid.Code != http.StatusPaymentRequired {
				t.Errorf("unpaid = %d", unpaid.Code)
				return
			}
			if paid := serve(rt, http.MethodGet, "/c", http.Header{"Authorization": {mitutest.MPP(t, unpaid.Header())}}); paid.Code != http.StatusCreated {
				t.Errorf("paid = %d", paid.Code)
			}
		})
	}
	wg.Wait()
	if rt.ran.Load() != 16 || len(f.Payments()) != 16 {
		t.Fatalf("ran %d, payments %d", rt.ran.Load(), len(f.Payments()))
	}
}
