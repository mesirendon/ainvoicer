package payments

import (
	"errors"
	"fmt"
	"time"

	"ap-agent/internal/money"
)

type Terms struct {
	NetDays           int
	EarlyDiscountBps  int
	EarlyDiscountDays int
}

type Schedule struct {
	PayOn         time.Time
	AmountMinor   int64
	DiscountMinor int64
	Reason        string
}

func ScheduleFor(invoiceAmountMinor int64, issue, due time.Time, terms Terms, today time.Time) (Schedule, error) {
	if invoiceAmountMinor <= 0 {
		return Schedule{}, fmt.Errorf("invoice amount must be positive, got %d", invoiceAmountMinor)
	}
	if issue.IsZero() {
		return Schedule{}, errors.New("issue date is required to schedule a payment")
	}

	if terms.EarlyDiscountBps > 0 && terms.EarlyDiscountDays > 0 {
		windowEnd := issue.AddDate(0, 0, terms.EarlyDiscountDays)
		if !today.After(windowEnd) {
			discount, err := money.ApplyBps(invoiceAmountMinor, terms.EarlyDiscountBps)
			if err != nil {
				return Schedule{}, fmt.Errorf("compute discount: %w", err)
			}
			return Schedule{
				PayOn:         windowEnd,
				AmountMinor:   invoiceAmountMinor - discount,
				DiscountMinor: discount,
				Reason: fmt.Sprintf("Early-payment discount of %s%% is open until %s; paying on the last day of the window.",
					money.FormatMinor(int64(terms.EarlyDiscountBps)), windowEnd.Format(time.DateOnly)),
			}, nil
		}
	}

	payOn := due
	reason := "Paying on the invoice due date."
	if payOn.IsZero() {
		payOn = issue.AddDate(0, 0, terms.NetDays)
		reason = fmt.Sprintf("No due date on the invoice; paying at net %d days from the issue date.", terms.NetDays)
	}
	if payOn.Before(today) {
		payOn = today
		reason = "The invoice is already overdue; paying today."
	}
	return Schedule{PayOn: payOn, AmountMinor: invoiceAmountMinor, Reason: reason}, nil
}
