package invoices

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ap-agent/internal/extraction"
	"ap-agent/internal/fx"
	"ap-agent/internal/geocode"
	"ap-agent/internal/matching"
	"ap-agent/internal/payments"
)

const invoiceID = "11111111-2222-4333-8444-555555555555"

type mocks struct {
	store     *MockStore
	extractor *MockExtractor
	geocoder  *MockGeocoder
}

func newService(t *testing.T) (*Service, mocks) {
	m := mocks{store: NewMockStore(t), extractor: NewMockExtractor(t), geocoder: NewMockGeocoder(t)}
	return &Service{
		Store:     m.store,
		Extractor: m.extractor,
		Geocoder:  m.geocoder,
		Rules: Rules{
			Tolerances:       matching.Tolerances{PriceBps: 200, OriginKM: 100},
			AutoApproveClean: true,
			FundingCurrency:  "COP",
		},
		ExtractTimeout: time.Second,
		GeocodeTimeout: time.Second,
	}, m
}

func extracted() extraction.ExtractedInvoice {
	return extraction.ExtractedInvoice{
		SupplierName:  "Acme",
		InvoiceNumber: "INV-1",
		PONumber:      "PO-1",
		Currency:      "USD",
		TotalMinor:    100000,
		ShipFrom:      extraction.Address{City: "Houston", PostalCode: "77002", Country: "US"},
		Lines:         []extraction.Line{{SKU: "A", Description: "Bearing", Quantity: 100, UnitPriceMinor: 1000, LineTotalMinor: 100000}},
	}
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func matchingInput(unitPrice int64) matching.Input {
	return matching.Input{
		Invoice: matching.Invoice{
			Number: "INV-1", PONumber: "PO-1", Currency: "USD", TotalMinor: 100 * unitPrice, ShipFromCountry: "US",
			Lines: []matching.Line{{LineNo: 1, SKU: "A", Quantity: 100, UnitPriceMinor: unitPrice, LineTotalMinor: 100 * unitPrice}},
		},
		Supplier: &matching.Supplier{ID: "s1", Name: "Acme"},
		PO: &matching.PO{ID: "po1", Number: "PO-1", SupplierID: "s1", Currency: "USD",
			Lines: []matching.POLine{{LineNo: 1, SKU: "A", Quantity: 100, UnitPriceMinor: 1000}}},
		Receipts: []matching.Receipt{{WarehouseCountry: "US", Lines: []matching.ReceiptLine{{SKU: "A", Quantity: 100}}}},
		Nearest:  &matching.NearestWarehouse{Name: "Houston DC", City: "Houston", DistanceKM: 2},
	}
}

func TestProcess_CleanInvoiceAutoApprovedAndScheduled(t *testing.T) {
	s, m := newService(t)
	houston := geocode.Point{Lat: 29.76, Lng: -95.37}
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.MatchedBy(func(f NewFile) bool {
		return f.FileName == "inv.pdf" && len(f.SHA256) == 64
	})).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", []byte("%PDF-1")).Return(extracted(), nil).Once()
	m.geocoder.EXPECT().Geocode(mock.Anything, geocode.Address{City: "Houston", PostalCode: "77002", Country: "US"}).Return(houston, nil).Once()
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, extracted(), &houston, s.Rules).
		RunAndReturn(func(_ context.Context, _ string, _ extraction.ExtractedInvoice, _ *geocode.Point, rules Rules) (string, error) {
			decision := rules.Decide(matchingInput(1000))
			require.True(t, decision.AutoApprove)
			plan, err := rules.PlanPayment(PaymentInput{
				InvoiceAmountMinor: 100000, Currency: "USD",
				IssueDate: day("2026-10-15"), DueDate: day("2026-11-14"), Today: day("2026-10-27"),
				Terms: payments.Terms{NetDays: 30},
				Rate:  &fx.Rate{Base: "USD", Quote: "COP", RateE8: 400000000000, SpreadBps: 100},
			})
			require.NoError(t, err)
			assert.Equal(t, day("2026-11-14"), plan.Schedule.PayOn)
			assert.Equal(t, int64(404000000), plan.Conversion.FundingAmountMinor)
			return StatusPaymentScheduled, nil
		}).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: StatusPaymentScheduled}, nil).Once()

	d, created, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, StatusPaymentScheduled, d.Status)
}

func TestDecide_WarningsAndAutoApproveOffWaitForHuman(t *testing.T) {
	rules := Rules{Tolerances: matching.Tolerances{PriceBps: 200, OriginKM: 100}, AutoApproveClean: true, FundingCurrency: "COP"}
	warned := rules.Decide(matchingInput(1010))
	assert.Equal(t, matching.VerdictMatchedWithWarnings, warned.Result.Verdict)
	assert.False(t, warned.AutoApprove)

	rules.AutoApproveClean = false
	clean := rules.Decide(matchingInput(1000))
	assert.Equal(t, matching.VerdictMatched, clean.Result.Verdict)
	assert.False(t, clean.AutoApprove)
}

func TestProcess_BlockedOnPriceVariance(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).Return(extracted(), nil).Once()
	m.geocoder.EXPECT().Geocode(mock.Anything, mock.Anything).Return(geocode.Point{}, nil).Once()
	var decision Decision
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ extraction.ExtractedInvoice, _ *geocode.Point, rules Rules) (string, error) {
			decision = rules.Decide(matchingInput(1080))
			require.Len(t, decision.Result.Findings, 1)
			assert.Equal(t, "PRICE_VARIANCE", decision.Result.Findings[0].Code)
			return string(decision.Result.Verdict), nil
		}).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: "BLOCKED"}, nil).Once()

	d, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
	assert.Equal(t, matching.VerdictBlocked, decision.Result.Verdict)
	assert.False(t, decision.AutoApprove)
	assert.Equal(t, "BLOCKED", d.Status)
}

func TestProcess_DuplicateFileReturnsExisting(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID}, nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: "MATCHED"}, nil).Once()

	d, created, err := s.Process(context.Background(), "again.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, invoiceID, d.ID)
}

func TestProcess_ReclaimedFileIsProcessedAgain(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).Return(extracted(), nil).Once()
	m.geocoder.EXPECT().Geocode(mock.Anything, mock.Anything).Return(geocode.Point{}, nil).Once()
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, mock.Anything, mock.Anything, mock.Anything).Return("MATCHED", nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: "MATCHED"}, nil).Once()

	_, created, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
	assert.False(t, created)
}

func TestProcess_ExtractionFailureAfterRetry(t *testing.T) {
	s, m := newService(t)
	invalid := fmt.Errorf("attempt 2: %w: currency must be a 3-letter ISO 4217 code", extraction.ErrInvalid)
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).Return(extraction.ExtractedInvoice{}, invalid).Once()
	m.store.EXPECT().MarkExtractionFailed(mock.Anything, invoiceID, invalid.Error()).Return(nil).Once()

	_, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	var extractionErr *ExtractionError
	require.ErrorAs(t, err, &extractionErr)
	assert.Equal(t, invoiceID, extractionErr.InvoiceID)
	assert.ErrorIs(t, err, extraction.ErrInvalid)
}

func TestProcess_ExtractionTimeout(t *testing.T) {
	s, m := newService(t)
	s.ExtractTimeout = time.Millisecond
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).
		RunAndReturn(func(ctx context.Context, _ string, _ []byte) (extraction.ExtractedInvoice, error) {
			<-ctx.Done()
			return extraction.ExtractedInvoice{}, ctx.Err()
		}).Once()
	m.store.EXPECT().MarkExtractionFailed(mock.Anything, invoiceID, context.DeadlineExceeded.Error()).Return(nil).Once()

	_, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, extraction.ErrInvalid)
}

func TestProcess_GeocodingFailureStillMatches(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).Return(extracted(), nil).Once()
	m.geocoder.EXPECT().Geocode(mock.Anything, mock.Anything).Return(geocode.Point{}, &geocode.GeocodingError{Reason: "blank"}).Once()
	in := matchingInput(1000)
	in.Nearest = nil
	var codes []string
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, extracted(), (*geocode.Point)(nil), mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ extraction.ExtractedInvoice, _ *geocode.Point, rules Rules) (string, error) {
			result := rules.Decide(in).Result
			for _, f := range result.Findings {
				codes = append(codes, f.Code)
			}
			return string(result.Verdict), nil
		}).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: "MATCHED_WITH_WARNINGS"}, nil).Once()

	d, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
	assert.Equal(t, []string{"ORIGIN_UNVERIFIED"}, codes)
	assert.Equal(t, "MATCHED_WITH_WARNINGS", d.Status)
}

func TestProcess_NoShipFromSkipsGeocoder(t *testing.T) {
	s, m := newService(t)
	inv := extracted()
	inv.ShipFrom = extraction.Address{}
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID, Created: true, Claimed: true}, nil).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).Return(inv, nil).Once()
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, inv, (*geocode.Point)(nil), mock.Anything).Return("MATCHED", nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID}, nil).Once()

	_, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
}

func TestProcess_ClientDisconnectDoesNotAbortProcessing(t *testing.T) {
	s, m := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, NewFile) (Received, error) {
			cancel()
			return Received{ID: invoiceID, Created: true, Claimed: true}, nil
		}).Once()
	m.extractor.EXPECT().Extract(mock.Anything, "inv.pdf", mock.Anything).
		RunAndReturn(func(ctx context.Context, _ string, _ []byte) (extraction.ExtractedInvoice, error) {
			require.NoError(t, ctx.Err())
			return extracted(), nil
		}).Once()
	m.geocoder.EXPECT().Geocode(mock.Anything, mock.Anything).Return(geocode.Point{}, nil).Once()
	m.store.EXPECT().ApplyVerdict(mock.Anything, invoiceID, mock.Anything, mock.Anything, mock.Anything).Return("MATCHED", nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID}, nil).Once()

	_, _, err := s.Process(ctx, "inv.pdf", []byte("%PDF-1"))

	require.NoError(t, err)
}

func TestProcess_StoreErrorIsReturned(t *testing.T) {
	s, m := newService(t)
	boom := errors.New("connection reset")
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{}, boom).Once()

	_, _, err := s.Process(context.Background(), "inv.pdf", []byte("%PDF-1"))

	assert.ErrorIs(t, err, boom)
}

func TestProcessSample_UnknownOrUnsafeName(t *testing.T) {
	s, _ := newService(t)
	s.SamplesDir = t.TempDir()
	for _, name := range []string{"missing", "../secret", "a/b", ""} {
		_, _, err := s.ProcessSample(context.Background(), name)
		assert.ErrorIs(t, err, ErrNotFound, name)
	}
}

func TestProcessSample_RunsThePipeline(t *testing.T) {
	s, m := newService(t)
	s.SamplesDir = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(s.SamplesDir, "01-clean.pdf"), []byte("%PDF-1"), 0o600))
	m.store.EXPECT().SaveReceivedInvoice(mock.Anything, mock.Anything).Return(Received{ID: invoiceID}, nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID}, nil).Once()

	samples, err := s.ListSamples(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Sample{{Name: "01-clean", FileName: "01-clean.pdf"}}, samples)

	_, _, err = s.ProcessSample(context.Background(), "01-clean")
	require.NoError(t, err)
}
