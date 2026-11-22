package mitu

import (
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^(\d+)(?:\.(\d+))?$`)

// decimal is a non-negative amount of money held exactly, as units/10^scale.
// Money never passes through floating point.
type decimal struct {
	units *big.Int
	scale int
}

func parseDecimal(s string) (decimal, bool) {
	m := decimalPattern.FindStringSubmatch(s)
	if m == nil {
		return decimal{}, false
	}
	units, ok := new(big.Int).SetString(m[1]+m[2], 10)
	return decimal{units: units, scale: len(m[2])}, ok
}

// parsePrice parses a price Mitu accepts: greater than 0, with at most 2
// decimal places.
func parsePrice(s string) (decimal, error) {
	d, ok := parseDecimal(s)
	if !ok || d.scale > 2 || d.sign() <= 0 {
		return decimal{}, errInvalidAmount
	}
	return d, nil
}

func (d decimal) cmp(o decimal) int {
	a, b := d.units, o.units
	switch {
	case d.scale < o.scale:
		a = new(big.Int).Mul(a, pow10(o.scale-d.scale))
	case d.scale > o.scale:
		b = new(big.Int).Mul(b, pow10(d.scale-o.scale))
	}
	return a.Cmp(b)
}

func (d decimal) sign() int { return d.units.Sign() }

// String returns the canonical form, without leading or trailing zeros, so
// "5000", "5000.00" and "05000" print, compare and cache as one price.
func (d decimal) String() string {
	digits := d.units.String()
	if d.scale == 0 {
		return digits
	}
	if len(digits) <= d.scale {
		digits = strings.Repeat("0", d.scale-len(digits)+1) + digits
	}
	whole, fraction := digits[:len(digits)-d.scale], strings.TrimRight(digits[len(digits)-d.scale:], "0")
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
