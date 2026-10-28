package payments

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestScheduleFor(t *testing.T) {
	twoTenNet30 := Terms{NetDays: 30, EarlyDiscountBps: 200, EarlyDiscountDays: 10}
	net30 := Terms{NetDays: 30}
	tests := []struct {
		name         string
		amount       int64
		issue, due   string
		terms        Terms
		today        string
		wantPayOn    string
		wantAmount   int64
		wantDiscount int64
	}{
		{"discount window open pays on last day", 19836000, "2026-10-24", "2026-11-23", twoTenNet30, "2026-10-27", "2026-11-03", 19439280, 396720},
		{"discount window last day pays today", 100000, "2026-10-24", "2026-11-23", twoTenNet30, "2026-11-03", "2026-11-03", 98000, 2000},
		{"discount window closed pays on due date", 100000, "2026-10-24", "2026-11-23", twoTenNet30, "2026-11-04", "2026-11-23", 100000, 0},
		{"discount rounds half up", 125, "2026-10-24", "", twoTenNet30, "2026-10-24", "2026-11-03", 122, 3},
		{"no discount terms pays on due date", 100000, "2026-10-15", "2026-11-14", net30, "2026-10-27", "2026-11-14", 100000, 0},
		{"missing due date uses net days", 100000, "2026-10-15", "", net30, "2026-10-27", "2026-11-14", 100000, 0},
		{"overdue pays today", 100000, "2026-09-01", "2026-10-01", net30, "2026-10-27", "2026-10-27", 100000, 0},
		{"overdue by net days pays today", 100000, "2026-09-01", "", net30, "2026-10-27", "2026-10-27", 100000, 0},
		{"due today pays today", 100000, "2026-09-27", "2026-10-27", net30, "2026-10-27", "2026-10-27", 100000, 0},
		{"discount bps without days is ignored", 100000, "2026-10-24", "2026-11-23", Terms{NetDays: 30, EarlyDiscountBps: 200}, "2026-10-24", "2026-11-23", 100000, 0},
		{"discount days without bps is ignored", 100000, "2026-10-24", "2026-11-23", Terms{NetDays: 30, EarlyDiscountDays: 10}, "2026-10-24", "2026-11-23", 100000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var due time.Time
			if tt.due != "" {
				due = day(tt.due)
			}
			got, err := ScheduleFor(tt.amount, day(tt.issue), due, tt.terms, day(tt.today))
			require.NoError(t, err)
			assert.Equal(t, day(tt.wantPayOn), got.PayOn)
			assert.Equal(t, tt.wantAmount, got.AmountMinor)
			assert.Equal(t, tt.wantDiscount, got.DiscountMinor)
			assert.Equal(t, tt.amount, got.AmountMinor+got.DiscountMinor)
			assert.NotEmpty(t, got.Reason)
		})
	}
}

func TestScheduleFor_Errors(t *testing.T) {
	_, err := ScheduleFor(0, day("2026-10-24"), time.Time{}, Terms{NetDays: 30}, day("2026-10-27"))
	require.Error(t, err)
	_, err = ScheduleFor(100, time.Time{}, time.Time{}, Terms{NetDays: 30}, day("2026-10-27"))
	require.Error(t, err)
}
