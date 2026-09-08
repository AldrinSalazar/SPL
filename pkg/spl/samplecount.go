package spl

import (
	"fmt"
	"math/big"
	"strings"
)

// SampleCount computes N = ceil(RATE * DURATION) using exact decimal arithmetic
// on the original duration token (including exponent notation). It does not
// derive the count from a rounded float64.
//
// The token must already match the general-number pattern. Token length and
// exponent bounds are enforced before constructing big integers.
func SampleCount(rate int, durationToken string, lim Limits) (int, *Diagnostic) {
	tok := durationToken
	if len(tok) > lim.MaxTokenLen {
		return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
			Message: fmt.Sprintf("duration token length %d exceeds limit %d", len(tok), lim.MaxTokenLen)}
	}
	// Split mantissa / exponent.
	mant := tok
	exp10 := 0
	if i := strings.IndexAny(tok, "eE"); i >= 0 {
		mant = tok[:i]
		es := tok[i+1:]
		if es == "" {
			return 0, &Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
				Message: fmt.Sprintf("malformed duration exponent in %q", tok)}
		}
		// Bound exponent digit length before parsing.
		digits := strings.TrimPrefix(strings.TrimPrefix(es, "+"), "-")
		if len(digits) > 7 {
			// Even 10^9999999 is absurd; check magnitude via length first.
			// Parse sign and compare against limit.
			neg := strings.HasPrefix(es, "-")
			_ = neg
			return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
				Message: fmt.Sprintf("duration exponent in %q exceeds limit ±%d", tok, lim.MaxExponentAbs)}
		}
		var neg bool
		v := 0
		s := es
		if strings.HasPrefix(s, "+") {
			s = s[1:]
		} else if strings.HasPrefix(s, "-") {
			neg = true
			s = s[1:]
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				return 0, &Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
					Message: fmt.Sprintf("malformed duration exponent in %q", tok)}
			}
			v = v*10 + int(c-'0')
		}
		if neg {
			v = -v
		}
		if v > lim.MaxExponentAbs || v < -lim.MaxExponentAbs {
			return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
				Message: fmt.Sprintf("duration exponent %d exceeds limit ±%d", v, lim.MaxExponentAbs)}
		}
		exp10 = v
	}
	neg := false
	if strings.HasPrefix(mant, "-") {
		neg = true
		mant = mant[1:]
	}
	if mant == "" {
		return 0, &Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
			Message: fmt.Sprintf("malformed duration %q", tok)}
	}
	intPart := mant
	fracPart := ""
	if i := strings.Index(mant, "."); i >= 0 {
		intPart = mant[:i]
		fracPart = mant[i+1:]
	}
	if intPart == "" {
		return 0, &Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
			Message: fmt.Sprintf("malformed duration %q", tok)}
	}
	digits := intPart + fracPart
	// Strip leading zeros.
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		// Zero duration.
		return 0, nil
	}
	if neg {
		// Negative duration: exact value negative; ceil <= 0.
		// Return 0 to signal invalid; caller reports DURATION_RANGE.
		return 0, nil
	}
	exp10 -= len(fracPart)
	// numerator = RATE * digits; value = numerator * 10^exp10; N = ceil(value).
	bigDigits, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0, &Diagnostic{Code: CodeDurationFormat, Field: "DURATION",
			Message: fmt.Sprintf("malformed duration %q", tok)}
	}
	numer := new(big.Int).Mul(big.NewInt(int64(rate)), bigDigits)
	if exp10 >= 0 {
		if exp10 > 0 {
			mult := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp10)), nil)
			numer.Mul(numer, mult)
		}
		if !numer.IsInt64() {
			return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
				Message: fmt.Sprintf("sample count exceeds representable range (RATE %d * DURATION %s)", rate, tok)}
		}
		v := numer.Int64()
		if v > int64(lim.MaxSamples) {
			return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
				Message: fmt.Sprintf("sample count %d exceeds limit %d (RATE %d * DURATION %s)", v, lim.MaxSamples, rate, tok)}
		}
		if v <= 0 {
			return 0, nil
		}
		// Guard int width (32-bit).
		if v > int64(int(^uint(0)>>1)) {
			return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
				Message: fmt.Sprintf("sample count %d exceeds platform int range", v)}
		}
		return int(v), nil
	}
	// exp10 < 0: N = ceil(numer / 10^(-exp10)).
	denomExp := -exp10
	// Fast path: if denom > numer then ceil = 1 (numer > 0).
	// Compare digit counts to avoid huge pow when possible.
	// digits(10^k) = k+1; digits(numer) approx len(numer.String()) but String is O(n).
	// For bounded exponents (<=10000) direct Exp is acceptable (10^10000 ~ 33k bits).
	denom := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(denomExp)), nil)
	q, r := new(big.Int).QuoRem(numer, denom, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	if q.Sign() <= 0 {
		// Positive duration always yields >= 1, but guard anyway.
		return 1, nil
	}
	if !q.IsInt64() {
		return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
			Message: fmt.Sprintf("sample count exceeds representable range (RATE %d * DURATION %s)", rate, tok)}
	}
	v := q.Int64()
	if v > int64(lim.MaxSamples) {
		return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
			Message: fmt.Sprintf("sample count %d exceeds limit %d (RATE %d * DURATION %s)", v, lim.MaxSamples, rate, tok)}
	}
	if v > int64(int(^uint(0)>>1)) {
		return 0, &Diagnostic{Code: CodeResourceLimit, Field: "DURATION",
			Message: fmt.Sprintf("sample count %d exceeds platform int range", v)}
	}
	return int(v), nil
}
