package invoices

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const reviewer = "reviewer"

var ErrReasonRequired = errors.New("a reason is required")

func (s *Service) Approve(ctx context.Context, id string) (Detail, error) {
	if err := s.Store.Approve(ctx, id, reviewer, s.Rules); err != nil {
		return Detail{}, err
	}
	return s.Store.GetInvoice(ctx, id)
}

func (s *Service) Override(ctx context.Context, id, reason string) (Detail, error) {
	reason, err := cleanReason(reason)
	if err != nil {
		return Detail{}, err
	}
	if err := s.Store.Override(ctx, id, reviewer, reason, s.Rules); err != nil {
		return Detail{}, err
	}
	return s.Store.GetInvoice(ctx, id)
}

func (s *Service) Reject(ctx context.Context, id, reason string) (Detail, error) {
	reason, err := cleanReason(reason)
	if err != nil {
		return Detail{}, err
	}
	if err := s.Store.Reject(ctx, id, reviewer, reason); err != nil {
		return Detail{}, err
	}
	return s.Store.GetInvoice(ctx, id)
}

func cleanReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", ErrReasonRequired
	}
	if utf8.RuneCountInString(reason) > 500 {
		return "", fmt.Errorf("%w: keep it under 500 characters", ErrReasonRequired)
	}
	return reason, nil
}
