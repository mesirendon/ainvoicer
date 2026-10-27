package matching

type Severity string

const (
	SeverityWarning Severity = "WARNING"
	SeverityBlock   Severity = "BLOCK"
)

type Verdict string

const (
	VerdictMatched             Verdict = "MATCHED"
	VerdictMatchedWithWarnings Verdict = "MATCHED_WITH_WARNINGS"
	VerdictBlocked             Verdict = "BLOCKED"
)

type Tolerances struct {
	PriceBps int
	OriginKM int
}

type Input struct {
	Invoice            Invoice
	Supplier           *Supplier
	PO                 *PO
	DuplicateOf        string
	Receipts           []Receipt
	PreviouslyInvoiced map[string]int64
	Nearest            *NearestWarehouse
}

type Invoice struct {
	Number          string
	PONumber        string
	SupplierName    string
	Currency        string
	TaxMinor        int64
	TotalMinor      int64
	ShipFromCity    string
	ShipFromCountry string
	Lines           []Line
}

type Line struct {
	LineNo         int
	SKU            string
	Description    string
	Quantity       int64
	UnitPriceMinor int64
	LineTotalMinor int64
}

type Supplier struct {
	ID   string
	Name string
}

type PO struct {
	ID           string
	Number       string
	SupplierID   string
	SupplierName string
	Currency     string
	Lines        []POLine
}

type POLine struct {
	LineNo         int
	SKU            string
	Description    string
	Quantity       int64
	UnitPriceMinor int64
}

type Receipt struct {
	WarehouseCountry string
	Lines            []ReceiptLine
}

type ReceiptLine struct {
	SKU      string
	Quantity int64
}

type NearestWarehouse struct {
	Name       string
	City       string
	DistanceKM float64
}

type Finding struct {
	LineNo   int
	Code     string
	Severity Severity
	Expected string
	Actual   string
	Message  string
}

type LineMatch struct {
	InvoiceLineNo int
	POLineNo      int
	POSKU         string
}

type Result struct {
	Verdict  Verdict
	Findings []Finding
	Lines    []LineMatch
}
