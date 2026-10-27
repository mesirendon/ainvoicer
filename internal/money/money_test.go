package money

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMinor(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"two decimals with thousands", "1,250.00", 125000, false},
		{"no decimals", "1250", 125000, false},
		{"one decimal", "12.5", 1250, false},
		{"zero", "0.00", 0, false},
		{"cents only", "0.07", 7, false},
		{"dollar symbol", "$1,250.00", 125000, false},
		{"US dollar symbol with space", "US$ 99.99", 9999, false},
		{"euro symbol", "€3.20", 320, false},
		{"surrounding spaces", "  42.10 ", 4210, false},
		{"millions", "12,345,678.90", 1234567890, false},
		{"max int64", "92,233,720,368,547,758.07", math.MaxInt64, false},
		{"three decimals rejected", "1.250", 0, true},
		{"four decimals rejected", "10.0000", 0, true},
		{"trailing dot rejected", "10.", 0, true},
		{"empty rejected", "", 0, true},
		{"symbol only rejected", "$", 0, true},
		{"negative rejected", "-5.00", 0, true},
		{"letters rejected", "12a.00", 0, true},
		{"european format rejected", "1.250,00", 0, true},
		{"bad grouping rejected", "12,50.00", 0, true},
		{"leading group too long rejected", "1250,000.00", 0, true},
		{"empty group rejected", "1,,250.00", 0, true},
		{"overflow rejected", "92,233,720,368,547,758.08", 0, true},
		{"two dots rejected", "1.2.3", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMinor(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatMinor(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0.00"},
		{"cents", 7, "0.07"},
		{"hundreds", 99999, "999.99"},
		{"thousands", 125000, "1,250.00"},
		{"millions", 1234567890, "12,345,678.90"},
		{"negative", -125000, "-1,250.00"},
		{"max int64", math.MaxInt64, "92,233,720,368,547,758.07"},
		{"min int64", math.MinInt64, "-92,233,720,368,547,758.08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatMinor(tt.in))
		})
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	for _, in := range []int64{0, 1, 10, 100, 123456, 100000000} {
		got, err := ParseMinor(FormatMinor(in))
		require.NoError(t, err)
		assert.Equal(t, in, got)
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"simple", 100, 250, 350, nil},
		{"zero", 0, 0, 0, nil},
		{"at max", math.MaxInt64 - 1, 1, math.MaxInt64, nil},
		{"overflow", math.MaxInt64, 1, 0, ErrOverflow},
		{"negative operand", 100, -250, 0, ErrNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Add(tt.a, tt.b)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int64
		want    int64
		wantErr error
	}{
		{"quantity times price", 200, 1250, 250000, nil},
		{"zero", 0, math.MaxInt64, 0, nil},
		{"overflow", math.MaxInt64/2 + 1, 2, 0, ErrOverflow},
		{"negative", -1, 5, 0, ErrNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Mul(tt.a, tt.b)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplyBps(t *testing.T) {
	tests := []struct {
		name    string
		amount  int64
		bps     int
		want    int64
		wantErr error
	}{
		{"two percent", 125000, 200, 2500, nil},
		{"rounds half up", 25, 200, 1, nil},
		{"rounds down below half", 24, 200, 0, nil},
		{"exact half", 50, 100, 1, nil},
		{"zero bps", 125000, 0, 0, nil},
		{"full amount", 125000, 10000, 125000, nil},
		{"negative amount", -1, 200, 0, ErrNegative},
		{"overflow", math.MaxInt64, 200, 0, ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyBps(tt.amount, tt.bps)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMulDivHalfUpBig(t *testing.T) {
	tests := []struct {
		name       string
		a, b, c, d int64
		want       int64
		wantErr    error
	}{
		{"usd to cop with 1 percent spread", 125000, 400000000000, 10100, 1000000000000, 505000000, nil},
		{"same currency", 125000, 100000000, 10000, 1000000000000, 125000, nil},
		{"rounds half up", 1, 1, 5, 10, 1, nil},
		{"rounds down below half", 1, 1, 4, 10, 0, nil},
		{"intermediate exceeds int64 but result fits", math.MaxInt64, 100000000, 10000, 1000000000000, math.MaxInt64, nil},
		{"result overflows", math.MaxInt64, 2, 1, 1, 0, ErrOverflow},
		{"negative", -1, 1, 1, 1, 0, ErrNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MulDivHalfUpBig(tt.a, tt.b, tt.c, tt.d)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
