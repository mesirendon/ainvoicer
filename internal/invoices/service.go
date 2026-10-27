package invoices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"ap-agent/internal/extraction"
	"ap-agent/internal/geocode"
	"ap-agent/internal/matching"
)

type Extractor interface {
	Extract(ctx context.Context, fileName string, pdf []byte) (extraction.ExtractedInvoice, error)
}

type Geocoder interface {
	Geocode(ctx context.Context, a geocode.Address) (geocode.Point, error)
}

type Store interface {
	SaveReceivedInvoice(ctx context.Context, f NewFile) (Received, error)
	MarkExtractionFailed(ctx context.Context, id, reason string) error
	ApplyVerdict(ctx context.Context, id string, inv extraction.ExtractedInvoice, shipFrom *geocode.Point, decide func(matching.Input) matching.Result) (matching.Verdict, error)
	GetInvoice(ctx context.Context, id string) (Detail, error)
	ListInvoices(ctx context.Context) ([]Summary, error)
	InvoiceFile(ctx context.Context, id string) (string, []byte, error)
}

type Service struct {
	Store          Store
	Extractor      Extractor
	Geocoder       Geocoder
	Tolerances     matching.Tolerances
	ExtractTimeout time.Duration
	GeocodeTimeout time.Duration
	SamplesDir     string
}

func (s *Service) Process(ctx context.Context, fileName string, pdf []byte) (Detail, bool, error) {
	sum := sha256.Sum256(pdf)
	rec, err := s.Store.SaveReceivedInvoice(ctx, NewFile{FileName: fileName, SHA256: hex.EncodeToString(sum[:]), Content: pdf})
	if err != nil {
		return Detail{}, false, err
	}
	if rec.Claimed {
		if err := s.process(context.WithoutCancel(ctx), rec.ID, fileName, pdf); err != nil {
			return Detail{}, false, err
		}
	}
	d, err := s.Store.GetInvoice(ctx, rec.ID)
	if err != nil {
		return Detail{}, false, err
	}
	return d, rec.Created, nil
}

func (s *Service) process(ctx context.Context, id, fileName string, pdf []byte) error {
	extractCtx, cancel := context.WithTimeout(ctx, s.ExtractTimeout)
	inv, err := s.Extractor.Extract(extractCtx, fileName, pdf)
	cancel()
	if err != nil {
		if markErr := s.Store.MarkExtractionFailed(ctx, id, err.Error()); markErr != nil {
			return fmt.Errorf("record extraction failure: %w", markErr)
		}
		return &ExtractionError{InvoiceID: id, Err: err}
	}

	shipFrom := s.geocode(ctx, id, inv.ShipFrom)
	if _, err := s.Store.ApplyVerdict(ctx, id, inv, shipFrom, s.decide); err != nil {
		return fmt.Errorf("apply verdict: %w", err)
	}
	return nil
}

func (s *Service) decide(in matching.Input) matching.Result {
	return matching.Match(in, s.Tolerances)
}

func (s *Service) geocode(ctx context.Context, id string, a extraction.Address) *geocode.Point {
	if a.IsEmpty() {
		return nil
	}
	geoCtx, cancel := context.WithTimeout(ctx, s.GeocodeTimeout)
	defer cancel()
	p, err := s.Geocoder.Geocode(geoCtx, geocode.Address{Line1: a.Line1, City: a.City, PostalCode: a.PostalCode, Country: a.Country})
	if err != nil {
		slog.WarnContext(ctx, "ship-from not geocoded", "invoice_id", id, "error", err)
		return nil
	}
	return &p
}

func (s *Service) Get(ctx context.Context, id string) (Detail, error) {
	return s.Store.GetInvoice(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	return s.Store.ListInvoices(ctx)
}

func (s *Service) File(ctx context.Context, id string) (string, []byte, error) {
	return s.Store.InvoiceFile(ctx, id)
}
