package money

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

var (
	ErrOverflow = errors.New("amount overflows int64")
	ErrNegative = errors.New("negative values are not supported")
)

var symbols = []string{"US$", "MX$", "$", "€", "£"}

func ParseMinor(s string) (int64, error) {
	original := s
	s = strings.TrimSpace(s)
	for _, sym := range symbols {
		if strings.HasPrefix(s, sym) {
			s = strings.TrimSpace(strings.TrimPrefix(s, sym))
			break
		}
	}
	if s == "" {
		return 0, fmt.Errorf("parse amount %q: empty", original)
	}

	whole, frac, hasDot := strings.Cut(s, ".")
	if hasDot && (len(frac) == 0 || len(frac) > 2) {
		return 0, fmt.Errorf("parse amount %q: expected 0 to 2 decimals", original)
	}
	if !validGrouping(whole) {
		return 0, fmt.Errorf("parse amount %q: invalid digits or thousands separators", original)
	}
	for len(frac) < 2 {
		frac += "0"
	}

	var n int64
	for _, r := range strings.ReplaceAll(whole, ",", "") + frac {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("parse amount %q: invalid character %q", original, r)
		}
		d := int64(r - '0')
		if n > (math.MaxInt64-d)/10 {
			return 0, fmt.Errorf("parse amount %q: %w", original, ErrOverflow)
		}
		n = n*10 + d
	}
	return n, nil
}

func validGrouping(whole string) bool {
	if whole == "" {
		return false
	}
	groups := strings.Split(whole, ",")
	for i, g := range groups {
		if g == "" || strings.Trim(g, "0123456789") != "" {
			return false
		}
		if len(groups) > 1 && (i == 0 && len(g) > 3 || i > 0 && len(g) != 3) {
			return false
		}
	}
	return true
}

func FormatMinor(minor int64) string {
	sign := ""
	units, cents := minor/100, minor%100
	if minor < 0 {
		sign = "-"
		units, cents = -units, -cents
	}
	whole := fmt.Sprintf("%d", units)

	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s%s.%02d", sign, b.String(), cents)
}

func Add(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, ErrNegative
	}
	if a > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func Mul(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, ErrNegative
	}
	if a != 0 && b > math.MaxInt64/a {
		return 0, ErrOverflow
	}
	return a * b, nil
}

func ApplyBps(amount int64, bps int) (int64, error) {
	if amount < 0 || bps < 0 {
		return 0, ErrNegative
	}
	product, err := Mul(amount, int64(bps))
	if err != nil {
		return 0, err
	}
	return divHalfUp(product, 10000), nil
}

func MulDivHalfUpBig(a, b, c, divisor int64) (int64, error) {
	if a < 0 || b < 0 || c < 0 {
		return 0, ErrNegative
	}
	if divisor <= 0 {
		return 0, errors.New("divisor must be positive")
	}
	product := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	product.Mul(product, big.NewInt(c))
	d := big.NewInt(divisor)
	q, r := new(big.Int).QuoRem(product, d, new(big.Int))
	if r.Lsh(r, 1).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, ErrOverflow
	}
	return q.Int64(), nil
}

func divHalfUp(n, d int64) int64 {
	q, r := n/d, n%d
	if r*2 >= d {
		q++
	}
	return q
}
