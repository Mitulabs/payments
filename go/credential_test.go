package mitu

import (
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func b64JSON(t *testing.T, enc *base64.Encoding, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return enc.EncodeToString(b)
}

func mppToken(t *testing.T, opaque any) string {
	t.Helper()
	return b64JSON(t, base64.RawURLEncoding, map[string]any{
		"challenge": map[string]any{"id": "ch_1", "realm": "api.mitu.ai", "method": "tempo", "intent": "charge", "request": "e30", "opaque": opaque},
		"payload":   map[string]string{"type": "transaction"},
	})
}

func x402Payload(t *testing.T, enc *base64.Encoding, resourceURL string) string {
	t.Helper()
	return b64JSON(t, enc, map[string]any{
		"x402Version": 2,
		"resource":    map[string]string{"url": resourceURL},
		"accepted":    map[string]string{"scheme": "exact", "network": "eip155:8453"},
		"payload":     map[string]string{"signature": "0x01"},
	})
}

func TestReadCredential(t *testing.T) {
	scope := b64JSON(t, base64.RawURLEncoding, map[string]string{"_mppx_scope": "pmt_abc123"})
	mpp := mppToken(t, scope)
	x402 := x402Payload(t, base64.StdEncoding, "https://api.mitu.ai/pay/pmt_abc123")

	tests := []struct {
		name        string
		header      http.Header
		wantForward http.Header
	}{
		{"mpp in Authorization", http.Header{"Authorization": {"Payment " + mpp}}, http.Header{"Authorization": {"Payment " + mpp}}},
		{"mpp scheme in lower case", http.Header{"Authorization": {"payment " + mpp}}, http.Header{"Authorization": {"Payment " + mpp}}},
		{"mpp after another scheme", http.Header{"Authorization": {"Bearer abc, Payment " + mpp}}, http.Header{"Authorization": {"Payment " + mpp}}},
		{"mpp in Payment-Authorization", http.Header{"Authorization": {"Bearer abc"}, "Payment-Authorization": {"Payment " + mpp}}, http.Header{"Authorization": {"Payment " + mpp}}},
		{"mpp opaque as an object", http.Header{"Authorization": {"Payment " + mppToken(t, map[string]string{"_mppx_scope": "pmt_abc123"})}}, nil},
		{"mpp padded base64url", http.Header{"Authorization": {"Payment " + b64JSON(t, base64.URLEncoding, map[string]any{"challenge": map[string]any{"opaque": scope}})}}, nil},
		{"x402 PAYMENT-SIGNATURE", http.Header{"Payment-Signature": {x402}}, http.Header{"Payment-Signature": {x402}}},
		{"x402 X-PAYMENT", http.Header{"X-Payment": {x402}}, http.Header{"X-Payment": {x402}}},
		{"x402 base64url", http.Header{"Payment-Signature": {x402Payload(t, base64.RawURLEncoding, "https://api.mitu.ai/pay/pmt_abc123")}}, nil},
		{"x402 resource with a query", http.Header{"Payment-Signature": {x402Payload(t, base64.StdEncoding, "https://agent.mitu.ai/pay/pmt_abc123?view=none")}}, nil},
		{"mpp preferred over x402", http.Header{"Authorization": {"Payment " + mpp}, "Payment-Signature": {x402}}, http.Header{"Authorization": {"Payment " + mpp}}},
		{"x402 used when mpp is unreadable", http.Header{"Authorization": {`Payment id="x", method="tempo"`}, "Payment-Signature": {x402}}, http.Header{"Payment-Signature": {x402}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header = tt.header
			cred, ok := readCredential(r)
			if !ok || cred.paymentID != "pmt_abc123" {
				t.Fatalf("readCredential = %+v, %v", cred, ok)
			}
			if tt.wantForward != nil && !maps.EqualFunc(cred.forward, tt.wantForward, slices.Equal[[]string]) {
				t.Errorf("forward = %v, want %v", cred.forward, tt.wantForward)
			}
		})
	}
}

func TestReadCredentialWithoutPaymentID(t *testing.T) {
	tests := map[string]http.Header{
		"not base64":            {"Authorization": {"Payment !!!"}},
		"not json":              {"Authorization": {"Payment " + base64.RawURLEncoding.EncodeToString([]byte("nope"))}},
		"no opaque":             {"Authorization": {"Payment " + mppToken(t, nil)}},
		"opaque without scope":  {"Authorization": {"Payment " + mppToken(t, b64JSON(t, base64.RawURLEncoding, map[string]string{"k": "v"}))}},
		"opaque not base64":     {"Authorization": {"Payment " + mppToken(t, "%%%")}},
		"scope not an id":       {"Authorization": {"Payment " + mppToken(t, map[string]string{"_mppx_scope": "../v1/public/payments"})}},
		"mitu parameters":       {"Authorization": {`Payment id="pmt_abc123", method="tempo"`}},
		"x402 garbage":          {"Payment-Signature": {"garbage"}},
		"x402 without resource": {"Payment-Signature": {b64JSON(t, base64.StdEncoding, map[string]any{"x402Version": 2})}},
		"x402 invoice resource": {"Payment-Signature": {x402Payload(t, base64.StdEncoding, "https://api.mitu.ai/invoice/inv_1")}},
		"x402 id with a slash":  {"Payment-Signature": {x402Payload(t, base64.StdEncoding, "https://api.mitu.ai/pay/a%2Fb")}},
		"x402 oversized":        {"Payment-Signature": {strings.Repeat("A", maxCredentialBytes+4)}},
	}
	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header = header
			cred, ok := readCredential(r)
			if !ok || cred.paymentID != "" {
				t.Fatalf("readCredential = %+v, %v; want a credential without a payment id", cred, ok)
			}
		})
	}
}

func TestReadCredentialAbsent(t *testing.T) {
	for _, auth := range []string{"", "Bearer abc", "Payment", "Payment "} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", auth)
		if _, ok := readCredential(r); ok {
			t.Errorf("Authorization %q read as a payment credential", auth)
		}
	}
}

func TestCheckAgentURL(t *testing.T) {
	for _, ok := range []string{"https://api.mitu.ai/pay/pmt_1", "http://127.0.0.1:8080/pay/pmt_1", "http://localhost/pay/pmt_1"} {
		if err := checkAgentURL(ok); err != nil {
			t.Errorf("checkAgentURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/pay/pmt_1", "http://api.mitu.ai/pay/pmt_1", "ftp://api.mitu.ai/x", "https://user:pw@api.mitu.ai/pay/pmt_1"} {
		if err := checkAgentURL(bad); err == nil {
			t.Errorf("checkAgentURL(%q) accepted", bad)
		}
	}
}
