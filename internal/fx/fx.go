package fx

import (
	"errors"
	"fmt"

	"ap-agent/internal/money"
)

const (
	RateScale = 100_000_000
	bpsScale  = 10_000
)

var ErrInvalidRate = errors.New("invalid fx rate")

type Rate struct {
	Base      string
	Quote     string
	RateE8    int64
	SpreadBps int
}

type Conversion struct {
	FundingCurrency    string
	RateE8             int64
	SpreadBps          int
	FundingAmountMinor int64
}

func Identity(currency string) Rate {
	return Rate{Base: currency, Quote: currency, RateE8: RateScale, SpreadBps: 0}
}

func Convert(amountMinor int64, r Rate) (Conversion, error) {
	if r.RateE8 <= 0 {
		return Conversion{}, fmt.Errorf("%w: rate_e8 must be positive, got %d", ErrInvalidRate, r.RateE8)
	}
	if r.SpreadBps < 0 {
		return Conversion{}, fmt.Errorf("%w: spread must not be negative, got %d bps", ErrInvalidRate, r.SpreadBps)
	}
	funding, err := money.MulDivHalfUpBig(amountMinor, r.RateE8, int64(bpsScale+r.SpreadBps), RateScale*bpsScale)
	if err != nil {
		return Conversion{}, fmt.Errorf("convert %d %s to %s: %w", amountMinor, r.Base, r.Quote, err)
	}
	return Conversion{FundingCurrency: r.Quote, RateE8: r.RateE8, SpreadBps: r.SpreadBps, FundingAmountMinor: funding}, nil
}
