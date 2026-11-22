package mitu

import "testing"

func mustDecimal(t *testing.T, s string) decimal {
	t.Helper()
	d, ok := parseDecimal(s)
	if !ok {
		t.Fatalf("parseDecimal(%q) failed", s)
	}
	return d
}

func TestParsePrice(t *testing.T) {
	for _, in := range []string{"1", "0.01", "12.5", "12.50", "05000", "5000.00"} {
		if _, err := parsePrice(in); err != nil {
			t.Errorf("parsePrice(%q) = %v", in, err)
		}
	}
	for _, in := range []string{"0", "0.00", "0.001", "1.005", "-1", "", "1e2"} {
		if _, err := parsePrice(in); err == nil {
			t.Errorf("parsePrice(%q) accepted", in)
		}
	}
}

func TestParseDecimalRejects(t *testing.T) {
	for _, in := range []string{"", "-1", "1e3", "1.", ".5", " 1", "1 ", "NaN", "１", "0x10", "+1"} {
		if _, ok := parseDecimal(in); ok {
			t.Errorf("parseDecimal(%q) accepted", in)
		}
	}
}

func TestDecimalCmp(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"5000", "5000.00", 0},
		{"3.21", "3.2100001", -1},
		{"10", "9.999999", 1},
		{"0.000", "0", 0},
	}
	for _, tt := range tests {
		if got := mustDecimal(t, tt.a).cmp(mustDecimal(t, tt.b)); got != tt.want {
			t.Errorf("cmp(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDecimalString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"05000.00", "5000"},
		{"12.50", "12.5"},
		{"0.000", "0"},
		{"0.05", "0.05"},
		{"000.000100", "0.0001"},
	}
	for _, tt := range tests {
		if got := mustDecimal(t, tt.in).String(); got != tt.want {
			t.Errorf("String(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
