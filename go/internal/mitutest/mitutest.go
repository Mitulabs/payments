// Package mitutest fakes Mitu for tests: its payments API and the agent_url
// each payment is paid at, on two httptest servers reached through an
// http.Client that routes Mitu's hosts to them, with helpers that answer its
// 402 the way MPP and x402 clients do.
package mitutest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	mitu "github.com/mitulabs/payments/go"
)

// APIKey is the key the fake API accepts.
const APIKey = "sk_test_abcdefghijklmnop"

// AgentHost is the host of the agent_url the fake gives each payment.
const AgentHost = "agent.mitu.test"

// Call is a request the fake received.
type Call struct {
	Agent  bool   // sent to agent_url rather than the API
	Host   string // the host the SDK addressed
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// Reply is a scripted answer.
type Reply struct {
	Status int
	Header http.Header
	Body   string
}

// Payment is a payment the fake holds.
type Payment struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Amount      string            `json:"amount"`
	Currency    string            `json:"currency"`
	Reference   string            `json:"reference"`
	Description string            `json:"description"`
	ExpiresAt   string            `json:"expires_at"`
	Metadata    map[string]string `json:"metadata"`
	AgentURL    string            `json:"agent_url"`
}

// Fake is a Mitu API and agent_url host.
type Fake struct {
	API   *httptest.Server
	Agent *httptest.Server

	transport *http.Transport

	mu       sync.Mutex
	calls    []Call
	payments map[string]*Payment
	apiReply *Reply
	settle   func(*Payment) *Reply
}

// New starts a Fake, closed when the test ends.
func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{payments: map[string]*Payment{}, transport: &http.Transport{}}
	f.API = httptest.NewServer(http.HandlerFunc(f.serveAPI))
	f.Agent = httptest.NewServer(http.HandlerFunc(f.serveAgent))
	t.Cleanup(func() {
		f.transport.CloseIdleConnections()
		f.API.Close()
		f.Agent.Close()
	})
	return f
}

// Client returns an http.Client that sends requests for api.mitu.ai and
// sandbox.mitu.ai to the fake API, and for AgentHost to the fake agent_url
// host, keeping the addressed host in the Host header. It refuses any other
// host, so a test never reaches the network.
func (f *Fake) Client() *http.Client {
	return &http.Client{Transport: roundTripFunc(f.route)}
}

// Mitu returns a Mitu using the fake through Client, with APIKey unless opts
// names a key.
func (f *Fake) Mitu(t testing.TB, opts mitu.Options) *mitu.Mitu {
	t.Helper()
	if opts.APIKey == "" {
		opts.APIKey = APIKey
	}
	opts.HTTPClient = f.Client()
	m, err := mitu.New(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// FailAPI makes every API call answer r; nil restores normal answers.
func (f *Fake) FailAPI(r *Reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.apiReply = r
}

// Settle overrides the answer to a paid agent_url request. A nil answer
// settles as Mitu does.
func (f *Fake) Settle(fn func(*Payment) *Reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settle = fn
}

// Calls returns the requests received so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Payments returns the payments held, in the order they were created.
func (f *Fake) Payments() []Payment {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Payment, 0, len(f.payments))
	for i := 1; i <= len(f.payments); i++ {
		out = append(out, *f.payments[paymentID(i)])
	}
	return out
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func (f *Fake) route(r *http.Request) (*http.Response, error) {
	var server *httptest.Server
	switch r.URL.Host {
	case "api.mitu.ai", "sandbox.mitu.ai":
		server = f.API
	case AgentHost:
		server = f.Agent
	default:
		return nil, fmt.Errorf("mitutest: request to unexpected host %q", r.URL.Host)
	}
	if r.URL.Scheme != "https" {
		return nil, fmt.Errorf("mitutest: request to %s over %s", r.URL.Host, r.URL.Scheme)
	}
	out := r.Clone(r.Context())
	out.Host = r.URL.Host
	out.URL.Scheme = "http"
	out.URL.Host = server.Listener.Addr().String()
	return f.transport.RoundTrip(out)
}

func paymentID(n int) string { return fmt.Sprintf("pmt_%020d", n) }

func (f *Fake) record(agent bool, r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, Call{Agent: agent, Host: r.Host, Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
	return body
}

func (f *Fake) serveAPI(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body := f.record(false, r)

	if f.apiReply != nil {
		write(w, *f.apiReply)
		return
	}
	if r.Header.Get("X-Api-Key") != APIKey {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Invalid API key"})
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/payments":
		var in struct {
			Amount      json.Number       `json:"amount"`
			Currency    string            `json:"currency"`
			Reference   string            `json:"reference"`
			Description string            `json:"description"`
			ExpiresAt   string            `json:"expires_at"`
			Metadata    map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid body"})
			return
		}
		id := paymentID(len(f.payments) + 1)
		p := &Payment{
			ID:          id,
			Status:      "pending",
			Amount:      twoPlaces(in.Amount.String()),
			Currency:    strings.ToUpper(in.Currency),
			Reference:   in.Reference,
			Description: in.Description,
			ExpiresAt:   in.ExpiresAt,
			Metadata:    in.Metadata,
			AgentURL:    "https://" + AgentHost + "/pay/" + id,
		}
		f.payments[id] = p
		writeJSON(w, http.StatusCreated, map[string]any{"message": "Payment created", "data": p})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/payments/"):
		p, ok := f.payments[strings.TrimPrefix(r.URL.Path, "/v1/payments/")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "Payment not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "Payment retrieved", "data": p})
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) serveAgent(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(true, r)

	p, ok := f.payments[strings.TrimPrefix(r.URL.Path, "/pay/")]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not found"})
		return
	}
	mpp := strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "payment ")
	if !mpp && r.Header.Get("Payment-Signature") == "" && r.Header.Get("X-Payment") == "" {
		write(w, challenge(p))
		return
	}
	if f.settle != nil {
		if reply := f.settle(p); reply != nil {
			write(w, *reply)
			return
		}
	}
	if p.Status != "pending" {
		write(w, Reply{
			Status: http.StatusConflict,
			Header: http.Header{"Content-Type": {"application/problem+json"}},
			Body:   `{"type":"https://paymentauth.org/problems/already-paid","status":409}`,
		})
		return
	}
	p.Status = "paid"
	method, network, header := "x402", "base", http.Header{
		"Payment-Response":   {"eDQwMi1yZWNlaXB0"},
		"X-Payment-Response": {"eDQwMi1yZWNlaXB0"},
	}
	if mpp {
		method, network, header = "mpp_crypto", "tempo", http.Header{"Payment-Receipt": {"bXBwLXJlY2VpcHQ"}}
	}
	header.Set("Content-Type", "application/json")
	header.Set("X-Internal", "secret")
	body, _ := json.Marshal(map[string]any{
		"status":        "paid",
		"resource_type": "payment",
		"resource_id":   p.ID,
		"receipt": map[string]any{
			"transaction_id": "txn_1",
			"tx_hash":        "0xabc",
			"amount_paid":    p.Amount,
			"currency":       p.Currency,
			"network":        network,
			"payment_method": method,
			"confirmed_at":   "2026-11-22T10:00:00.000Z",
		},
	})
	write(w, Reply{Status: http.StatusOK, Header: header, Body: string(body)})
}

// challenge is Mitu's 402 for p: an MPP charge challenge bound to the
// payment id through opaque, as mppx encodes a scope, and an x402 v2
// PAYMENT-REQUIRED naming agent_url as the resource.
func challenge(p *Payment) Reply {
	opaque := b64url(map[string]string{"_mppx_scope": p.ID})
	request := b64url(map[string]string{"amount": "500000", "currency": "0x20c0", "recipient": "0x1111"})
	www := fmt.Sprintf(`Payment id="ch_%s", realm="api.mitu.ai", method="tempo", intent="charge", request="%s", expires="2026-11-22T10:05:00Z", opaque="%s"`, p.ID, request, opaque)
	required, _ := json.Marshal(map[string]any{
		"x402Version": 2,
		"resource":    map[string]string{"url": p.AgentURL, "description": p.Description, "mimeType": "application/json"},
		"accepts": []map[string]any{{
			"scheme": "exact", "network": "eip155:8453", "amount": "500000",
			"asset": "0x8335", "payTo": "0x1111", "maxTimeoutSeconds": 300,
		}},
	})
	return Reply{
		Status: http.StatusPaymentRequired,
		Header: http.Header{
			"Content-Type":     {"application/problem+json"},
			"Www-Authenticate": {www},
			"Payment-Required": {base64.StdEncoding.EncodeToString(required)},
			"Cache-Control":    {"no-store"},
			"X-Internal":       {"secret"},
			"Set-Cookie":       {"mitu=1"},
		},
		Body: fmt.Sprintf(`{"type":"https://paymentauth.org/problems/payment-required","status":402,"resource_id":%q}`, p.ID),
	}
}

var authParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// MPP answers Mitu's 402 as an MPP client does: it echoes the challenge in
// WWW-Authenticate, opaque included, and returns the Authorization value.
func MPP(t testing.TB, h http.Header) string {
	t.Helper()
	www := h.Get("Www-Authenticate")
	if www == "" {
		t.Fatal("no WWW-Authenticate challenge")
	}
	params := map[string]string{}
	for _, m := range authParam.FindAllStringSubmatch(www, -1) {
		params[m[1]] = m[2]
	}
	wire, _ := json.Marshal(map[string]any{
		"challenge": params,
		"payload":   map[string]string{"type": "transaction", "signature": "0xsigned"},
		"source":    "did:pkh:eip155:4217:0x2222222222222222222222222222222222222222",
	})
	return "Payment " + base64.RawURLEncoding.EncodeToString(wire)
}

// X402 answers Mitu's 402 as an x402 v2 client does: a PaymentPayload
// echoing the resource and the first accepted requirements, as the
// PAYMENT-SIGNATURE value.
func X402(t testing.TB, h http.Header) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(h.Get("Payment-Required"))
	if err != nil {
		t.Fatalf("PAYMENT-REQUIRED: %v", err)
	}
	var required struct {
		Resource json.RawMessage   `json:"resource"`
		Accepts  []json.RawMessage `json:"accepts"`
	}
	if err := json.Unmarshal(raw, &required); err != nil || len(required.Accepts) == 0 {
		t.Fatalf("PAYMENT-REQUIRED: %s", raw)
	}
	payload, _ := json.Marshal(map[string]any{
		"x402Version": 2,
		"resource":    required.Resource,
		"accepted":    required.Accepts[0],
		"payload":     map[string]any{"signature": "0xsigned", "authorization": map[string]string{"from": "0x2222"}},
	})
	return base64.StdEncoding.EncodeToString(payload)
}

func b64url(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func twoPlaces(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	return whole + "." + (frac + "00")[:2]
}

func write(w http.ResponseWriter, r Reply) {
	for k, v := range r.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(r.Status)
	_, _ = io.WriteString(w, r.Body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	write(w, Reply{Status: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: string(b)})
}
