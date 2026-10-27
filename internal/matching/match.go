package matching

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"ap-agent/internal/money"
)

func Match(in Input, tol Tolerances) Result {
	var r Result
	inv := in.Invoice

	if in.Supplier == nil {
		r.block(0, "UNKNOWN_SUPPLIER", "a supplier in the master data", inv.SupplierName,
			fmt.Sprintf("Supplier %q is not in the supplier master data.", inv.SupplierName))
	}
	if in.DuplicateOf != "" {
		r.block(0, "DUPLICATE_INVOICE", "a new invoice number", inv.Number,
			fmt.Sprintf("Invoice %s from this supplier was already received (invoice %s).", inv.Number, in.DuplicateOf))
	}
	if in.PO == nil {
		if inv.PONumber == "" {
			r.block(0, "NO_PO", "a purchase order number", "none",
				"The invoice does not reference a purchase order.")
		} else {
			r.block(0, "NO_PO", "an existing purchase order", inv.PONumber,
				fmt.Sprintf("Purchase order %s was not found.", inv.PONumber))
		}
		return r.finish()
	}
	po := in.PO
	if in.Supplier != nil && po.SupplierID != in.Supplier.ID {
		r.block(0, "PO_SUPPLIER_MISMATCH", po.SupplierName, in.Supplier.Name,
			fmt.Sprintf("Purchase order %s belongs to %s, not to %s.", po.Number, po.SupplierName, in.Supplier.Name))
	}
	sameCurrency := inv.Currency == po.Currency
	if !sameCurrency {
		r.block(0, "CURRENCY_MISMATCH", po.Currency, inv.Currency,
			fmt.Sprintf("Invoice currency %s does not match purchase order currency %s.", inv.Currency, po.Currency))
	}

	r.checkLines(in, tol, sameCurrency)
	r.checkTotals(inv)
	r.checkOrigin(in, tol)
	return r.finish()
}

func (r *Result) checkLines(in Input, tol Tolerances, sameCurrency bool) {
	inv, po := in.Invoice, in.PO
	billed := map[int]int64{}
	for _, l := range inv.Lines {
		poLine, ok := findPOLine(po.Lines, l)
		if !ok {
			r.block(l.LineNo, "LINE_NOT_ON_PO", "a line on "+po.Number, describe(l),
				fmt.Sprintf("Line %d (%s) does not match any line on purchase order %s.", l.LineNo, describe(l), po.Number))
			continue
		}
		r.Lines = append(r.Lines, LineMatch{InvoiceLineNo: l.LineNo, POLineNo: poLine.LineNo, POSKU: poLine.SKU})
		if sameCurrency {
			r.checkPrice(l, poLine, inv.Currency, tol.PriceBps)
		}
		billed[poLine.LineNo] = addCapped(billed[poLine.LineNo], l.Quantity)
	}

	for _, poLine := range po.Lines {
		thisInvoice, ok := billed[poLine.LineNo]
		if !ok {
			continue
		}
		received := receivedQuantity(in.Receipts, poLine.SKU)
		already := in.PreviouslyInvoiced[poLine.SKU]
		total := addCapped(already, thisInvoice)
		if total <= received {
			continue
		}
		r.block(firstInvoiceLine(r.Lines, poLine.LineNo), "QTY_EXCEEDS_RECEIVED",
			fmt.Sprintf("at most %d", max(received-already, 0)), fmt.Sprintf("%d", thisInvoice),
			fmt.Sprintf("SKU %s: ordered %d, received %d, already invoiced %d, this invoice %d. Billing exceeds the received quantity by %d.",
				poLine.SKU, poLine.Quantity, received, already, thisInvoice, total-received))
	}
}

func (r *Result) checkPrice(l Line, poLine POLine, currency string, toleranceBps int) {
	if l.UnitPriceMinor == poLine.UnitPriceMinor {
		return
	}
	diff := l.UnitPriceMinor - poLine.UnitPriceMinor
	direction := "above"
	if diff < 0 {
		diff = -diff
		direction = "below"
	}
	expected := money.FormatMinor(poLine.UnitPriceMinor) + " " + currency
	actual := money.FormatMinor(l.UnitPriceMinor) + " " + currency
	variance := "an unmeasurable amount"
	if bps, err := money.Mul(diff, 10000); err == nil {
		variance = money.FormatMinor(bps/poLine.UnitPriceMinor) + "%"
	}
	tolerance := money.FormatMinor(int64(toleranceBps)) + "%"

	if withinTolerance(diff, poLine.UnitPriceMinor, toleranceBps) {
		r.warn(l.LineNo, "PRICE_VARIANCE_WITHIN_TOLERANCE", expected, actual,
			fmt.Sprintf("Unit price %s for %s is %s %s the PO price %s, within the %s tolerance.", actual, poLine.SKU, variance, direction, expected, tolerance))
		return
	}
	r.block(l.LineNo, "PRICE_VARIANCE", expected, actual,
		fmt.Sprintf("Unit price %s for %s is %s %s the PO price %s, beyond the %s tolerance.", actual, poLine.SKU, variance, direction, expected, tolerance))
}

func withinTolerance(diff, poPrice int64, toleranceBps int) bool {
	scaledDiff, err := money.Mul(diff, 10000)
	if err != nil {
		return false
	}
	allowed, err := money.Mul(poPrice, int64(toleranceBps))
	if err != nil {
		return false
	}
	return scaledDiff <= allowed
}

func (r *Result) checkTotals(inv Invoice) {
	sum := inv.TaxMinor
	for _, l := range inv.Lines {
		var err error
		if sum, err = money.Add(sum, l.LineTotalMinor); err != nil {
			r.warn(0, "TOTAL_MISMATCH", "line totals plus tax", "overflow", "Line totals plus tax are too large to add up.")
			return
		}
	}
	diff := sum - inv.TotalMinor
	if diff < 0 {
		diff = -diff
	}
	if diff <= int64(len(inv.Lines)) {
		return
	}
	r.warn(0, "TOTAL_MISMATCH", money.FormatMinor(sum), money.FormatMinor(inv.TotalMinor),
		fmt.Sprintf("Line totals plus tax add up to %s %s, but the invoice total is %s %s.",
			money.FormatMinor(sum), inv.Currency, money.FormatMinor(inv.TotalMinor), inv.Currency))
}

func (r *Result) checkOrigin(in Input, tol Tolerances) {
	inv := in.Invoice
	if inv.ShipFromCountry == "" || len(in.Receipts) == 0 {
		return
	}
	var countries []string
	for _, rc := range in.Receipts {
		if !slices.Contains(countries, rc.WarehouseCountry) {
			countries = append(countries, rc.WarehouseCountry)
		}
	}
	if !slices.Contains(countries, inv.ShipFromCountry) {
		r.block(0, "ORIGIN_COUNTRY_MISMATCH", strings.Join(countries, ", "), inv.ShipFromCountry,
			fmt.Sprintf("Goods ship from %s but were received from a warehouse in %s.", inv.ShipFromCountry, strings.Join(countries, ", ")))
		return
	}
	if in.Nearest == nil {
		r.warn(0, "ORIGIN_UNVERIFIED", "a geocoded ship-from address", inv.ShipFromCity,
			"The ship-from address could not be geocoded, so its distance to the receiving warehouse was not checked.")
		return
	}
	if in.Nearest.DistanceKM <= float64(tol.OriginKM) {
		return
	}
	r.warn(0, "ORIGIN_MISMATCH", fmt.Sprintf("within %d km of %s", tol.OriginKM, in.Nearest.City),
		fmt.Sprintf("%.0f km", in.Nearest.DistanceKM),
		fmt.Sprintf("Ship-from %s is %.0f km from the receiving warehouse %s (%s), more than the %d km tolerance.",
			inv.ShipFromCity, in.Nearest.DistanceKM, in.Nearest.Name, in.Nearest.City, tol.OriginKM))
}

func (r *Result) finish() Result {
	r.Verdict = VerdictMatched
	for _, f := range r.Findings {
		if f.Severity == SeverityBlock {
			r.Verdict = VerdictBlocked
			return *r
		}
		r.Verdict = VerdictMatchedWithWarnings
	}
	return *r
}

func (r *Result) block(lineNo int, code, expected, actual, message string) {
	r.Findings = append(r.Findings, Finding{LineNo: lineNo, Code: code, Severity: SeverityBlock, Expected: expected, Actual: actual, Message: message})
}

func (r *Result) warn(lineNo int, code, expected, actual, message string) {
	r.Findings = append(r.Findings, Finding{LineNo: lineNo, Code: code, Severity: SeverityWarning, Expected: expected, Actual: actual, Message: message})
}

func findPOLine(lines []POLine, l Line) (POLine, bool) {
	if l.SKU != "" {
		for _, p := range lines {
			if strings.EqualFold(p.SKU, l.SKU) {
				return p, true
			}
		}
	}
	for _, p := range lines {
		if normalize(p.Description) == normalize(l.Description) {
			return p, true
		}
	}
	return POLine{}, false
}

func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func receivedQuantity(receipts []Receipt, sku string) int64 {
	var total int64
	for _, rc := range receipts {
		for _, l := range rc.Lines {
			if strings.EqualFold(l.SKU, sku) {
				total = addCapped(total, l.Quantity)
			}
		}
	}
	return total
}

func addCapped(a, b int64) int64 {
	sum, err := money.Add(a, b)
	if err != nil {
		return math.MaxInt64
	}
	return sum
}

func firstInvoiceLine(matches []LineMatch, poLineNo int) int {
	for _, m := range matches {
		if m.POLineNo == poLineNo {
			return m.InvoiceLineNo
		}
	}
	return 0
}

func describe(l Line) string {
	if l.SKU == "" {
		return fmt.Sprintf("%q", l.Description)
	}
	return fmt.Sprintf("SKU %s, %q", l.SKU, l.Description)
}
