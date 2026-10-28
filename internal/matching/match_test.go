package matching

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tol = Tolerances{PriceBps: 200, OriginKM: 100}

func cleanInput() Input {
	return Input{
		Invoice: Invoice{
			Number:          "INV-1",
			PONumber:        "PO-1",
			SupplierName:    "Acme",
			Currency:        "USD",
			TaxMinor:        0,
			TotalMinor:      300000,
			ShipFromCity:    "Houston",
			ShipFromCountry: "US",
			Lines: []Line{
				{LineNo: 1, SKU: "A", Description: "Bearing", Quantity: 100, UnitPriceMinor: 1000, LineTotalMinor: 100000},
				{LineNo: 2, SKU: "B", Description: "Hose", Quantity: 50, UnitPriceMinor: 4000, LineTotalMinor: 200000},
			},
		},
		Supplier: &Supplier{ID: "s1", Name: "Acme"},
		PO: &PO{ID: "po1", Number: "PO-1", SupplierID: "s1", SupplierName: "Acme", Currency: "USD", Lines: []POLine{
			{LineNo: 1, SKU: "A", Description: "Bearing", Quantity: 200, UnitPriceMinor: 1000},
			{LineNo: 2, SKU: "B", Description: "Hose", Quantity: 50, UnitPriceMinor: 4000},
		}},
		Receipts: []Receipt{
			{WarehouseCountry: "US", Lines: []ReceiptLine{{SKU: "A", Quantity: 100}, {SKU: "B", Quantity: 50}}},
		},
		PreviouslyInvoiced: map[string]int64{},
		Nearest:            &NearestWarehouse{Name: "Houston DC", City: "Houston", DistanceKM: 3},
	}
}

func codes(r Result) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Code)
	}
	return out
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Input)
		wantVerdict Verdict
		wantCodes   []string
	}{
		{"clean invoice", func(*Input) {}, VerdictMatched, nil},
		{"unknown supplier", func(in *Input) { in.Supplier = nil }, VerdictBlocked, []string{"UNKNOWN_SUPPLIER"}},
		{"po number missing", func(in *Input) { in.PO = nil; in.Invoice.PONumber = "" }, VerdictBlocked, []string{"NO_PO"}},
		{"po not found", func(in *Input) { in.PO = nil }, VerdictBlocked, []string{"NO_PO"}},
		{"unknown supplier and no po", func(in *Input) { in.Supplier = nil; in.PO = nil }, VerdictBlocked, []string{"UNKNOWN_SUPPLIER", "NO_PO"}},
		{"duplicate without po still flagged", func(in *Input) { in.PO = nil; in.DuplicateOf = "inv-0" }, VerdictBlocked, []string{"DUPLICATE_INVOICE", "NO_PO"}},
		{"po of another supplier", func(in *Input) { in.PO.SupplierID = "s2"; in.PO.SupplierName = "Other" }, VerdictBlocked, []string{"PO_SUPPLIER_MISMATCH"}},
		{"duplicate invoice number", func(in *Input) { in.DuplicateOf = "inv-0" }, VerdictBlocked, []string{"DUPLICATE_INVOICE"}},
		{"currency mismatch skips price checks", func(in *Input) {
			in.Invoice.Currency = "MXN"
			in.Invoice.Lines[0].UnitPriceMinor = 99999
			in.Invoice.Lines[0].LineTotalMinor = 9999900
			in.Invoice.TotalMinor = 9999900 + 200000
		}, VerdictBlocked, []string{"CURRENCY_MISMATCH"}},
		{"line not on po", func(in *Input) {
			in.Invoice.Lines[1].SKU = "Z"
			in.Invoice.Lines[1].Description = "Unknown widget"
		}, VerdictBlocked, []string{"LINE_NOT_ON_PO"}},
		{"sku missing matches by normalized description", func(in *Input) {
			in.Invoice.Lines[1].SKU = ""
			in.Invoice.Lines[1].Description = "  HOSE "
		}, VerdictMatched, nil},
		{"sku case insensitive", func(in *Input) { in.Invoice.Lines[0].SKU = "a" }, VerdictMatched, nil},
		{"price 1 percent above warns", func(in *Input) { setPrice(in, 1010) }, VerdictMatchedWithWarnings, []string{"PRICE_VARIANCE_WITHIN_TOLERANCE"}},
		{"price exactly at 2 percent warns", func(in *Input) { setPrice(in, 1020) }, VerdictMatchedWithWarnings, []string{"PRICE_VARIANCE_WITHIN_TOLERANCE"}},
		{"price one cent beyond 2 percent blocks", func(in *Input) { setPrice(in, 1021) }, VerdictBlocked, []string{"PRICE_VARIANCE"}},
		{"price 8 percent above blocks", func(in *Input) { setPrice(in, 1080) }, VerdictBlocked, []string{"PRICE_VARIANCE"}},
		{"price below po also checked", func(in *Input) { setPrice(in, 900) }, VerdictBlocked, []string{"PRICE_VARIANCE"}},
		{"price one cent above warns", func(in *Input) { setPrice(in, 1001) }, VerdictMatchedWithWarnings, []string{"PRICE_VARIANCE_WITHIN_TOLERANCE"}},
		{"quantity equal to received", func(in *Input) {}, VerdictMatched, nil},
		{"quantity below received is partial billing", func(in *Input) { setQuantity(in, 0, 40) }, VerdictMatched, nil},
		{"quantity above received blocks", func(in *Input) { setQuantity(in, 0, 101) }, VerdictBlocked, []string{"QTY_EXCEEDS_RECEIVED"}},
		{"previously invoiced counts", func(in *Input) { in.PreviouslyInvoiced["A"] = 1 }, VerdictBlocked, []string{"QTY_EXCEEDS_RECEIVED"}},
		{"received across two partial receipts", func(in *Input) {
			in.Receipts = []Receipt{
				{WarehouseCountry: "US", Lines: []ReceiptLine{{SKU: "A", Quantity: 60}, {SKU: "B", Quantity: 50}}},
				{WarehouseCountry: "US", Lines: []ReceiptLine{{SKU: "A", Quantity: 40}}},
			}
		}, VerdictMatched, nil},
		{"two invoice lines for the same po line are summed", func(in *Input) {
			in.Invoice.Lines[1] = Line{LineNo: 2, SKU: "A", Description: "Bearing", Quantity: 1, UnitPriceMinor: 1000, LineTotalMinor: 1000}
			in.Invoice.TotalMinor = 101000
		}, VerdictBlocked, []string{"QTY_EXCEEDS_RECEIVED"}},
		{"huge quantity does not overflow", func(in *Input) { in.PreviouslyInvoiced["A"] = math.MaxInt64 }, VerdictBlocked, []string{"QTY_EXCEEDS_RECEIVED"}},
		{"total off by one cent per line is tolerated", func(in *Input) { in.Invoice.TotalMinor = 300002 }, VerdictMatched, nil},
		{"total off beyond tolerance warns", func(in *Input) { in.Invoice.TotalMinor = 300003 }, VerdictMatchedWithWarnings, []string{"TOTAL_MISMATCH"}},
		{"tax included in total", func(in *Input) { in.Invoice.TaxMinor = 48000; in.Invoice.TotalMinor = 348000 }, VerdictMatched, nil},
		{"origin within tolerance", func(in *Input) { in.Nearest.DistanceKM = 100 }, VerdictMatched, nil},
		{"origin beyond tolerance warns", func(in *Input) { in.Nearest.DistanceKM = 100.5 }, VerdictMatchedWithWarnings, []string{"ORIGIN_MISMATCH"}},
		{"origin in another country blocks", func(in *Input) { in.Invoice.ShipFromCountry = "MX" }, VerdictBlocked, []string{"ORIGIN_COUNTRY_MISMATCH"}},
		{"origin skipped when ship-from absent", func(in *Input) { in.Invoice.ShipFromCountry = ""; in.Nearest = nil }, VerdictMatched, nil},
		{"origin skipped without receipts", func(in *Input) {
			in.Receipts = nil
			in.Nearest = nil
			in.Invoice.Lines = in.Invoice.Lines[:0]
			in.Invoice.TotalMinor = 0
		}, VerdictMatched, nil},
		{"origin unverified when not geocoded", func(in *Input) { in.Nearest = nil }, VerdictMatchedWithWarnings, []string{"ORIGIN_UNVERIFIED"}},
		{"warnings and blocks give blocked", func(in *Input) { setPrice(in, 1010); in.DuplicateOf = "x" }, VerdictBlocked, []string{"DUPLICATE_INVOICE", "PRICE_VARIANCE_WITHIN_TOLERANCE"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := cleanInput()
			tt.mutate(&in)
			got := Match(in, tol)
			assert.Equal(t, tt.wantVerdict, got.Verdict)
			assert.Equal(t, tt.wantCodes, codes(got))
		})
	}
}

func TestMatch_ZeroToleranceBlocksAnyVariance(t *testing.T) {
	in := cleanInput()
	setPrice(&in, 1001)
	got := Match(in, Tolerances{PriceBps: 0, OriginKM: 100})
	assert.Equal(t, []string{"PRICE_VARIANCE"}, codes(got))
}

func TestMatch_Messages(t *testing.T) {
	in := cleanInput()
	setPrice(&in, 1080)
	setQuantity(&in, 1, 60)
	in.Nearest = &NearestWarehouse{Name: "Dallas DC", City: "Dallas", DistanceKM: 2206.4}
	in.Invoice.ShipFromCity = "New York"

	got := Match(in, tol)

	require.Len(t, got.Findings, 3)
	price, qty, origin := got.Findings[0], got.Findings[1], got.Findings[2]
	assert.Equal(t, 1, price.LineNo)
	assert.Equal(t, "10.00 USD", price.Expected)
	assert.Equal(t, "10.80 USD", price.Actual)
	assert.Equal(t, "Unit price 10.80 USD for A is 8.00% above the PO price 10.00 USD, beyond the 2.00% tolerance.", price.Message)
	assert.Equal(t, 2, qty.LineNo)
	assert.Equal(t, "SKU B: ordered 50, received 50, already invoiced 0, this invoice 60. Billing exceeds the received quantity by 10.", qty.Message)
	assert.Equal(t, "Ship-from New York is 2206 km from the receiving warehouse Dallas DC (Dallas), more than the 100 km tolerance.", origin.Message)
}

func TestMatch_DuplicateMessageHasNoInternalID(t *testing.T) {
	in := cleanInput()
	in.DuplicateOf = "11111111-2222-4333-8444-555555555555"
	got := Match(in, tol)
	require.NotEmpty(t, got.Findings)
	assert.Equal(t, "Invoice INV-1 from this supplier was already received; this is a second copy.", got.Findings[0].Message)
}

func TestMatch_QuantityExpectedNeverNegative(t *testing.T) {
	in := cleanInput()
	in.PreviouslyInvoiced["A"] = 120
	got := Match(in, tol)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, "at most 0", got.Findings[0].Expected)
}

func TestMatch_LineMatches(t *testing.T) {
	in := cleanInput()
	in.Invoice.Lines[1].SKU = ""
	got := Match(in, tol)
	assert.Equal(t, []LineMatch{
		{InvoiceLineNo: 1, POLineNo: 1, POSKU: "A"},
		{InvoiceLineNo: 2, POLineNo: 2, POSKU: "B"},
	}, got.Lines)
}

func setPrice(in *Input, price int64) {
	l := &in.Invoice.Lines[0]
	in.Invoice.TotalMinor += l.Quantity*price - l.LineTotalMinor
	l.UnitPriceMinor = price
	l.LineTotalMinor = l.Quantity * price
}

func setQuantity(in *Input, i int, qty int64) {
	l := &in.Invoice.Lines[i]
	in.Invoice.TotalMinor += qty*l.UnitPriceMinor - l.LineTotalMinor
	l.Quantity = qty
	l.LineTotalMinor = qty * l.UnitPriceMinor
}
