package mitu

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type environment string

const (
	sandbox    environment = "sandbox"
	production environment = "production"
)

var apiKeyPattern = regexp.MustCompile(`^sk_(test|live)_[A-Za-z0-9]{16,}$`)

// defaultBaseURL is the Mitu API, shared by live and test keys.
const defaultBaseURL = "https://api.mitu.ai/economy"

// resolveEnvironment derives the environment from the key prefix: sk_test_
// is sandbox, sk_live_ is production. An explicit base URL is for tests and
// must use https unless it points at this machine.
func resolveEnvironment(apiKey, baseURL string) (environment, string, error) {
	m := apiKeyPattern.FindStringSubmatch(apiKey)
	if m == nil {
		return "", "", fmt.Errorf("%w: APIKey must be a Mitu secret key (sk_test_... or sk_live_...)", ErrConfig)
	}
	env := sandbox
	if m[1] == "live" {
		env = production
	}
	if baseURL == "" {
		return env, defaultBaseURL, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("%w: BaseURL is not a valid URL", ErrConfig)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocal(u.Hostname())) {
		return "", "", fmt.Errorf("%w: BaseURL must use https", ErrConfig)
	}
	return env, u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), nil
}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
