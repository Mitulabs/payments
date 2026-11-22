package mitu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Version is the version of this SDK.
const Version = "0.1.0"

const (
	sdkName          = "github.com/mitulabs/payments/go@" + Version
	userAgent        = "mitulabs-payments-go/" + Version
	callTimeout      = 10 * time.Second
	maxResponseBytes = 1 << 20
)

var errRedirect = errors.New("redirect refused")

// client calls the Mitu API, with the key in x-api-key, and the agent_url
// of a payment, without it. Every call is bounded by callTimeout and no
// redirect is followed, so the key never reaches a host Mitu did not name.
type client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newClient(apiKey, baseURL string, hc *http.Client) *client {
	if hc == nil {
		hc = &http.Client{}
	}
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirect }
	return &client{baseURL: baseURL, apiKey: apiKey, http: &c}
}

// payment is the part of Mitu's PaymentView this package reads.
type payment struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Amount    string            `json:"amount"`
	Currency  string            `json:"currency"`
	Reference string            `json:"reference"`
	Metadata  map[string]string `json:"metadata"`
	AgentURL  string            `json:"agent_url"`
}

type createPayment struct {
	Amount      json.Number       `json:"amount"`
	Currency    string            `json:"currency"`
	Reference   string            `json:"reference"`
	Description string            `json:"description,omitempty"`
	ExpiresAt   string            `json:"expires_at"`
	Metadata    map[string]string `json:"metadata"`
}

func (c *client) createPayment(ctx context.Context, body createPayment) (payment, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return payment{}, fmt.Errorf("mitu: encoding payment: %w", err)
	}
	return c.payment(ctx, http.MethodPost, "/v1/public/payments", b)
}

func (c *client) getPayment(ctx context.Context, id string) (payment, error) {
	return c.payment(ctx, http.MethodGet, "/v1/public/payments/"+url.PathEscape(id), nil)
}

func (c *client) payment(ctx context.Context, method, path string, body []byte) (payment, error) {
	op := method + " " + path
	header := http.Header{"X-Api-Key": {c.apiKey}}
	if body != nil {
		header.Set("Content-Type", "application/json")
	}
	resp, err := c.do(ctx, method, c.baseURL+path, header, body, op)
	if err != nil {
		return payment{}, err
	}
	if resp.status < 200 || resp.status > 299 {
		return payment{}, &apiError{status: resp.status, message: fmt.Sprintf("%s failed (%d)", op, resp.status)}
	}
	var envelope struct {
		Data payment `json:"data"`
	}
	if err := json.Unmarshal(resp.body, &envelope); err != nil || envelope.Data.ID == "" || envelope.Data.AgentURL == "" {
		return payment{}, &apiError{status: resp.status, message: op + " returned an unreadable payment"}
	}
	if err := checkAgentURL(envelope.Data.AgentURL); err != nil {
		return payment{}, &apiError{status: resp.status, message: op + " returned an unusable agent_url"}
	}
	return envelope.Data, nil
}

// agent calls a payment's agent_url with only the given headers: never the
// API key, and never anything else from the agent's request.
func (c *client) agent(ctx context.Context, agentURL string, header http.Header) (*response, error) {
	return c.do(ctx, http.MethodGet, agentURL, header.Clone(), nil, "GET agent_url")
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (c *client) do(ctx context.Context, method, target string, header http.Header, body []byte, op string) (*response, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	var payload io.Reader
	if body != nil {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return nil, &apiError{message: "building " + op, err: err}
	}
	req.Header = header
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, errRedirect) && resp != nil {
			return nil, &apiError{status: resp.StatusCode, message: fmt.Sprintf("%s redirected (%d)", op, resp.StatusCode)}
		}
		return nil, &apiError{message: op + " unreachable", err: err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, &apiError{message: op + " unreachable", err: err}
	}
	return &response{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

// checkAgentURL accepts an absolute https URL, or http on this machine.
func checkAgentURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return errors.New("not an absolute URL")
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLocal(u.Hostname())) {
		return nil
	}
	return errors.New("not https")
}
