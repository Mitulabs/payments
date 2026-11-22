package mitu

import (
	"errors"
	"strings"
	"testing"
)

func TestNewUsesBaseURLOfKey(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		liveAPIKey: "https://api.mitu.ai",
		testAPIKey: "https://sandbox.mitu.ai",
	} {
		m, err := New(t.Context(), Options{APIKey: key})
		if err != nil {
			t.Fatal(err)
		}
		if got := m.client.baseURL; got != want {
			t.Errorf("%s: base URL = %s, want %s", key[:8], got, want)
		}
	}
}

func TestResolveBaseURLRejectsKey(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"",
		"sk_test_",
		"sk_live_short",
		"sk_test_abc-defghijklmnop",
		"pk_live_Zq81Vx0Lm2Pp7Rr4",
		"sk_prod_Zq81Vx0Lm2Pp7Rr4",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			_, err := resolveBaseURL(key)
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("err = %v, want ErrConfig", err)
			}
			if secret := key[min(len(key), 8):]; secret != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("error echoes the key: %v", err)
			}
		})
	}
}
