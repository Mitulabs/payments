package mitu

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const (
	headerAuthorization        = "Authorization"
	headerPaymentAuthorization = "Payment-Authorization"
	headerPaymentSignature     = "Payment-Signature"
	headerXPayment             = "X-Payment"

	// mppScopeKey is the opaque metadata key mppx binds a challenge's scope
	// to. Mitu's scope is the payment id.
	mppScopeKey = "_mppx_scope"

	maxCredentialBytes = 64 << 10
)

var paymentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)

// credential is a payment credential found on a request.
type credential struct {
	// paymentID is the Mitu payment it answers, or "" when none could be
	// read from it.
	paymentID string
	// forward holds the one header that carries it to agent_url.
	forward http.Header
}

// readCredential returns the request's payment credential and whether it
// carries one at all. An MPP credential (Authorization or
// Payment-Authorization) is preferred over an x402 one (PAYMENT-SIGNATURE,
// or X-PAYMENT); only the one used is forwarded.
func readCredential(r *http.Request) (credential, bool) {
	tokens := paymentTokens(r.Header, headerAuthorization, headerPaymentAuthorization)
	x402Name, x402Value := x402Header(r.Header)
	if len(tokens) == 0 && x402Value == "" {
		return credential{}, false
	}
	for _, token := range tokens {
		if id := mppPaymentID(token); id != "" {
			return credential{paymentID: id, forward: http.Header{headerAuthorization: {"Payment " + token}}}, true
		}
	}
	if id := x402PaymentID(x402Value); id != "" {
		return credential{paymentID: id, forward: http.Header{x402Name: {x402Value}}}, true
	}
	return credential{}, true
}

// paymentTokens returns the token of every Payment scheme in the named
// headers, which may each list several schemes separated by commas.
func paymentTokens(h http.Header, names ...string) []string {
	var tokens []string
	for _, name := range names {
		for _, value := range h.Values(name) {
			for part := range strings.SplitSeq(value, ",") {
				scheme, token, ok := strings.Cut(strings.TrimSpace(part), " ")
				if ok && strings.EqualFold(scheme, "Payment") {
					tokens = append(tokens, strings.TrimSpace(token))
				}
			}
		}
	}
	return tokens
}

func x402Header(h http.Header) (string, string) {
	if v := h.Get(headerPaymentSignature); v != "" {
		return headerPaymentSignature, v
	}
	return headerXPayment, h.Get(headerXPayment)
}

// mppPaymentID reads the payment id from an MPP credential token: base64url
// JSON whose challenge echoes the opaque parameter, itself base64url JSON
// holding the scope. mppx and mpp-go send opaque as that string; a decoded
// object is accepted too, as mpp-go's parser does.
func mppPaymentID(token string) string {
	var wire struct {
		Challenge struct {
			Opaque json.RawMessage `json:"opaque"`
		} `json:"challenge"`
	}
	if !decodeJSON(token, &wire) {
		return ""
	}
	var meta map[string]string
	var encoded string
	switch {
	case json.Unmarshal(wire.Challenge.Opaque, &encoded) == nil:
		if !decodeJSON(encoded, &meta) {
			return ""
		}
	case json.Unmarshal(wire.Challenge.Opaque, &meta) != nil:
		return ""
	}
	return validPaymentID(meta[mppScopeKey])
}

// x402PaymentID reads the payment id from an x402 v2 PaymentPayload: the
// echoed resource URL is the payment's agent_url, ending /pay/<id>.
func x402PaymentID(header string) string {
	var payload struct {
		Resource struct {
			URL string `json:"url"`
		} `json:"resource"`
	}
	if header == "" || !decodeJSON(header, &payload) {
		return ""
	}
	u, err := url.Parse(payload.Resource.URL)
	if err != nil {
		return ""
	}
	dir, id := path.Split(u.Path)
	if path.Base(dir) != "pay" {
		return ""
	}
	return validPaymentID(id)
}

func validPaymentID(id string) string {
	if !paymentIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// decodeJSON decodes base64 JSON in any of the four base64 alphabets and
// paddings clients use.
func decodeJSON(s string, v any) bool {
	if len(s) > maxCredentialBytes {
		return false
	}
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return json.Unmarshal(b, v) == nil
		}
	}
	return false
}
