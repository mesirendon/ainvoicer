package fx

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ap-agent/internal/money"
)

func TestConvert(t *testing.T) {
	usdCop := Rate{Base: "USD", Quote: "COP", RateE8: 400000000000, SpreadBps: 100}
	tests := []struct {
		name    string
		amount  int64
		rate    Rate
		want    int64
		wantErr error
	}{
		{"usd to cop with 1 percent spread", 125000, usdCop, 505000000, nil},
		{"usd to cop without spread", 125000, Rate{Base: "USD", Quote: "COP", RateE8: 400000000000}, 500000000, nil},
		{"same currency is identity", 125000, Identity("COP"), 125000, nil},
		{"mxn to cop 1.5 percent spread", 19836000, Rate{Base: "MXN", Quote: "COP", RateE8: 22000000000, SpreadBps: 150}, 4429378800, nil},
		{"fractional rate rounds half up", 1, Rate{Base: "A", Quote: "B", RateE8: 50000000}, 1, nil},
		{"fractional rate rounds down below half", 1, Rate{Base: "A", Quote: "B", RateE8: 49999999}, 0, nil},
		{"zero amount", 0, usdCop, 0, nil},
		{"result overflows int64", math.MaxInt64, usdCop, 0, money.ErrOverflow},
		{"zero rate", 100, Rate{RateE8: 0}, 0, ErrInvalidRate},
		{"negative spread", 100, Rate{RateE8: RateScale, SpreadBps: -1}, 0, ErrInvalidRate},
		{"negative amount", -1, usdCop, 0, money.ErrNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Convert(tt.amount, tt.rate)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.FundingAmountMinor)
			assert.Equal(t, tt.rate.Quote, got.FundingCurrency)
			assert.Equal(t, tt.rate.RateE8, got.RateE8)
			assert.Equal(t, tt.rate.SpreadBps, got.SpreadBps)
		})
	}
}
