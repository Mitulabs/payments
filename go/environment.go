package mitu

import (
	"fmt"
	"regexp"
)

var apiKeyPattern = regexp.MustCompile(`^sk_(test|live)_[A-Za-z0-9]{16,}$`)

// baseURLs is the Mitu API for each kind of key: production for live keys
// and the sandbox for test keys.
var baseURLs = map[string]string{
	"live": "https://api.mitu.ai",
	"test": "https://sandbox.mitu.ai",
}

// resolveBaseURL returns the Mitu API origin for apiKey, which the key alone
// decides.
func resolveBaseURL(apiKey string) (string, error) {
	m := apiKeyPattern.FindStringSubmatch(apiKey)
	if m == nil {
		return "", fmt.Errorf("%w: APIKey must be a Mitu secret key (sk_test_... or sk_live_...)", ErrConfig)
	}
	return baseURLs[m[1]], nil
}
