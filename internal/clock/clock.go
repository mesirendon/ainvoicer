package clock

import (
	"context"
	"errors"
)

var ErrInvalidDays = errors.New("days must be between 1 and 365")

type Clock struct {
	Today      string `json:"today"`
	OffsetDays int    `json:"offsetDays"`
}

type Store interface {
	DemoClock(ctx context.Context) (Clock, error)
	AdvanceDemoClock(ctx context.Context, days int) (Clock, error)
	ResetDemo(ctx context.Context) error
}

type Service struct {
	Store Store
}

func (s *Service) Now(ctx context.Context) (Clock, error) {
	return s.Store.DemoClock(ctx)
}

func (s *Service) Advance(ctx context.Context, days int) (Clock, error) {
	if days < 1 || days > 365 {
		return Clock{}, ErrInvalidDays
	}
	return s.Store.AdvanceDemoClock(ctx, days)
}

func (s *Service) Reset(ctx context.Context) (Clock, error) {
	if err := s.Store.ResetDemo(ctx); err != nil {
		return Clock{}, err
	}
	return s.Store.DemoClock(ctx)
}
