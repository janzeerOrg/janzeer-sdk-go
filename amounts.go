package janzeer

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Amounts cross this SDK as decimal STRINGS ("1.5", "0.01") and are scaled to an integer of base units exactly the
// way the node does. No function here takes or returns a float: 0.1 is not representable in binary, and money must
// not depend on how a float prints.

var (
	decimalRe = regexp.MustCompile(`^-?(\d+)(?:\.(\d+))?$`)
	scale     = new(big.Int).Exp(big.NewInt(10), big.NewInt(Decimals), nil)
)

// NormalizeAmount checks that s is a plain decimal (no exponent, no "+") and returns it trimmed.
func NormalizeAmount(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !decimalRe.MatchString(s) {
		return "", fmt.Errorf("janzeer: not a decimal amount: %q", s)
	}
	return s, nil
}

// IsValidAmount reports whether s is a plain decimal with at most 8 fractional digits (what the node accepts).
func IsValidAmount(s string) bool {
	n, err := NormalizeAmount(s)
	if err != nil {
		return false
	}
	_, frac, _ := strings.Cut(n, ".")
	return len(frac) <= Decimals
}

// ToScaled is the node's toScaledLong: BigDecimal(amount).setScale(8, HALF_UP) x 10^8 as an integer of base units.
// Pinned by the "scaled" table of the conformance vectors.
func ToScaled(amount string) (*big.Int, error) {
	s, err := NormalizeAmount(amount)
	if err != nil {
		return nil, err
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	frac8 := (frac + "00000000")[:Decimals]
	v, _ := new(big.Int).SetString(whole+frac8, 10)
	if len(frac) > Decimals && frac[Decimals] >= '5' { // HALF_UP on the first dropped digit
		v.Add(v, big.NewInt(1))
	}
	if neg {
		v.Neg(v)
	}
	return v, nil
}

// FromScaled is the inverse of ToScaled: base units -> a decimal string with exactly 8 fractional digits.
func FromScaled(scaled *big.Int) string {
	abs := new(big.Int).Abs(scaled)
	whole, frac := new(big.Int).QuoRem(abs, scale, new(big.Int))
	sign := ""
	if scaled.Sign() < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s%s.%08s", sign, whole.String(), frac.String())
}

// FormatOptions controls FormatJNZ. The zero value means: 8 decimals, trimmed, with the ticker, ungrouped.
type FormatOptions struct {
	// Decimals to keep (0 means the default 8; use NoDecimals for none). Digits beyond are truncated, never rounded.
	Decimals int
	// KeepZeros keeps trailing zeros instead of trimming them.
	KeepZeros bool
	// NoTicker omits the " JNZ" suffix.
	NoTicker bool
	// Group separates thousands with ",".
	Group bool
}

// NoDecimals asks FormatJNZ for whole coins only.
const NoDecimals = -1

// FormatJNZ renders an amount for people: FormatJNZ("1.50000000", FormatOptions{}) == "1.5 JNZ".
func FormatJNZ(amount string, o FormatOptions) (string, error) {
	s, err := NormalizeAmount(amount)
	if err != nil {
		return "", err
	}
	decimals := o.Decimals
	if decimals == 0 {
		decimals = Decimals
	} else if decimals < 0 {
		decimals = 0
	}
	neg := strings.HasPrefix(s, "-")
	whole, frac, _ := strings.Cut(strings.TrimPrefix(s, "-"), ".")
	if len(frac) > decimals {
		frac = frac[:decimals]
	}
	if o.KeepZeros {
		frac += strings.Repeat("0", decimals-len(frac))
	} else {
		frac = strings.TrimRight(frac, "0")
	}
	if o.Group {
		var b strings.Builder
		for i, c := range whole {
			if i > 0 && (len(whole)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(c)
		}
		whole = b.String()
	}
	out := whole
	if frac != "" {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	if !o.NoTicker {
		out += " " + Ticker
	}
	return out, nil
}

var tickerSuffix = regexp.MustCompile(`(?i)\s*` + Ticker + `\s*$`)

// ParseJNZ turns user input like "1,234.5 JNZ" into a canonical amount string ("1234.5").
func ParseJNZ(input string) (string, error) {
	return NormalizeAmount(strings.ReplaceAll(tickerSuffix.ReplaceAllString(input, ""), ",", ""))
}

// CompareAmounts compares two amounts exactly: -1, 0 or 1.
func CompareAmounts(a, b string) (int, error) {
	x, err := ToScaled(a)
	if err != nil {
		return 0, err
	}
	y, err := ToScaled(b)
	if err != nil {
		return 0, err
	}
	return x.Cmp(y), nil
}

// AddAmounts returns a + b exactly, as a canonical 8-decimal string.
func AddAmounts(a, b string) (string, error) { return arith(a, b, (*big.Int).Add) }

// SubAmounts returns a - b exactly, as a canonical 8-decimal string.
func SubAmounts(a, b string) (string, error) { return arith(a, b, (*big.Int).Sub) }

func arith(a, b string, op func(z, x, y *big.Int) *big.Int) (string, error) {
	x, err := ToScaled(a)
	if err != nil {
		return "", err
	}
	y, err := ToScaled(b)
	if err != nil {
		return "", err
	}
	return FromScaled(op(new(big.Int), x, y)), nil
}
