package mitu

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveEnvironment(t *testing.T) {
	tests := []struct {
		name, key, baseURL string
		wantEnv            environment
		wantURL            string
	}{
		{"test key", testAPIKey, "", sandbox, "https://api.mitu.ai/economy"},
		{"live key", liveAPIKey, "", production, "https://api.mitu.ai/economy"},
		{"https override keeps the path", testAPIKey, "https://mitu.test/economy/", sandbox, "https://mitu.test/economy"},
		{"localhost over http", testAPIKey, "http://localhost:4000", sandbox, "http://localhost:4000"},
		{"loopback over http", testAPIKey, "http://127.0.0.1:4000", sandbox, "http://127.0.0.1:4000"},
		{"ipv6 loopback over http", testAPIKey, "http://[::1]:4000", sandbox, "http://[::1]:4000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, url, err := resolveEnvironment(tt.key, tt.baseURL)
			if err != nil {
				t.Fatal(err)
			}
			if env != tt.wantEnv || url != tt.wantURL {
				t.Errorf("got (%s, %s), want (%s, %s)", env, url, tt.wantEnv, tt.wantURL)
			}
		})
	}
}

func TestResolveEnvironmentRejectsKey(t *testing.T) {
	for _, key := range []string{
		"",
		"sk_test_",
		"sk_live_short",
		"sk_test_abc-defghijklmnop",
		"pk_live_Zq81Vx0Lm2Pp7Rr4",
		"sk_prod_Zq81Vx0Lm2Pp7Rr4",
	} {
		t.Run(key, func(t *testing.T) {
			_, _, err := resolveEnvironment(key, "")
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("err = %v, want ErrConfig", err)
			}
			if secret := key[min(len(key), 8):]; secret != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("error echoes the key: %v", err)
			}
		})
	}
}

func TestResolveEnvironmentRejectsBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"http://api.mitu.ai",
		"http://localhost.evil.com",
		"ftp://localhost",
		"not a url",
		"https://",
	} {
		t.Run(baseURL, func(t *testing.T) {
			if _, _, err := resolveEnvironment(testAPIKey, baseURL); !errors.Is(err, ErrConfig) {
				t.Fatalf("err = %v, want ErrConfig", err)
			}
		})
	}
}
