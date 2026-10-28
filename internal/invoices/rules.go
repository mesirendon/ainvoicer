package invoices

import (
	"errors"
	"fmt"
	"time"

	"ap-agent/internal/fx"
	"ap-agent/internal/matching"
	"ap-agent/internal/payments"
)

var ErrCannotSchedule = errors.New("cannot schedule a payment")

type Rules struct {
	Tolerances       matching.Tolerances
	AutoApproveClean bool
	FundingCurrency  string
}

type Decision struct {
	Result      matching.Result
	AutoApprove bool
}

type PaymentInput struct {
	InvoiceAmountMinor int64
	Currency           string
	IssueDate          time.Time
	DueDate            time.Time
	Terms              payments.Terms
	Today              time.Time
	Rate               *fx.Rate
}

type PaymentPlan struct {
	Currency   string
	Schedule   payments.Schedule
	Conversion fx.Conversion
}

func (r Rules) Decide(in matching.Input) Decision {
	result := matching.Match(in, r.Tolerances)
	return Decision{
		Result:      result,
		AutoApprove: r.AutoApproveClean && result.Verdict == matching.VerdictMatched,
	}
}

func (r Rules) PlanPayment(in PaymentInput) (PaymentPlan, error) {
	schedule, err := payments.ScheduleFor(in.InvoiceAmountMinor, in.IssueDate, in.DueDate, in.Terms, in.Today)
	if err != nil {
		return PaymentPlan{}, fmt.Errorf("%w: %w", ErrCannotSchedule, err)
	}
	if schedule.AmountMinor <= 0 {
		return PaymentPlan{}, fmt.Errorf("%w: the amount after discount is %d", ErrCannotSchedule, schedule.AmountMinor)
	}
	rate := fx.Identity(r.FundingCurrency)
	if in.Currency != r.FundingCurrency {
		if in.Rate == nil {
			return PaymentPlan{}, fmt.Errorf("%w: no fx rate from %s to %s", ErrCannotSchedule, in.Currency, r.FundingCurrency)
		}
		rate = *in.Rate
	}
	conversion, err := fx.Convert(schedule.AmountMinor, rate)
	if err != nil {
		return PaymentPlan{}, fmt.Errorf("%w: %w", ErrCannotSchedule, err)
	}
	if conversion.FundingAmountMinor <= 0 {
		return PaymentPlan{}, fmt.Errorf("%w: the funding amount rounds to %d", ErrCannotSchedule, conversion.FundingAmountMinor)
	}
	return PaymentPlan{Currency: in.Currency, Schedule: schedule, Conversion: conversion}, nil
}
