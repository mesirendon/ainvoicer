package main

import "time"

type party struct {
	name, taxID, line1, city, postalCode, country string
}

type item struct {
	sku, description string
	quantity         int64
	unitPriceMinor   int64
}

type sample struct {
	file          string
	layout        string
	supplier      party
	shipFrom      party
	invoiceNumber string
	poNumber      string
	currency      string
	issueOffset   int
	netDays       int
	terms         string
	taxBps        int
	items         []item
}

var buyer = party{"Andes Retail S.A.S.", "NIT 900.123.456-7", "Carrera 7 # 71-21", "Bogotá", "110231", "CO"}

var (
	acme      = party{"Acme Industrial Supply Inc.", "US-84-1234567", "1200 Smith Street", "Houston", "77002", "US"}
	hanse     = party{"Hanse Präzisionsteile GmbH", "DE811234567", "Großer Grasbrook 9", "Hamburg", "20095", "DE"}
	textiles  = party{"Textiles del Norte S.A. de C.V.", "TNO980512AB3", "Av. Constitución 400", "Monterrey", "64000", "MX"}
	pacific   = party{"Pacific Coast Electronics LLC", "US-95-7654321", "350 S Grand Ave", "Los Angeles", "90071", "US"}
	lakeshore = party{"Lakeshore Packaging Co.", "US-36-5550123", "233 S Wacker Dr", "Chicago", "60606", "US"}
	gulf      = party{"Gulf Marine Parts Corp.", "US-59-8889900", "701 Brickell Ave", "Miami", "33131", "US"}

	acmeDallas  = party{line1: "2200 Ross Ave", city: "Dallas", postalCode: "75201", country: "US"}
	acmeNewYork = party{line1: "350 5th Ave", city: "New York", postalCode: "10118", country: "US"}
)

var samples = []sample{
	{"01-clean-usd", "classic", acme, acme, "ACM-2026-0412", "PO-1001", "USD", -12, 30, "Net 30", 0, []item{
		{"ACM-100", "Industrial ball bearing 6204", 200, 1250},
		{"ACM-200", "Hydraulic hose 1/2 in x 10 ft", 50, 4800},
		{"ACM-300", "Nitrile safety gloves, box of 100", 30, 2275},
	}},
	{"02-factura-mxn-discount", "factura", textiles, textiles, "TDN-F-00871", "PO-2001", "MXN", -3, 30, "2/10 neto 30", 1600, []item{
		{"TDN-01", "Tela de algodón, rollo de 50 m", 40, 325000},
		{"TDN-02", "Hilo de poliéster, caja", 100, 41000},
	}},
	{"03-price-variance", "modern", acme, acmeDallas, "ACM-2026-0431", "PO-1002", "USD", -10, 30, "Net 30", 0, []item{
		{"ACM-400", "Galvanized steel bracket", 500, 432},
	}},
	{"04-qty-exceeds-received", "modern", acme, acmeDallas, "ACM-2026-0432", "PO-1002", "USD", -9, 30, "Net 30", 0, []item{
		{"ACM-600", "Pneumatic control valve", 80, 6500},
	}},
	{"05-duplicate-number", "modern", acme, acme, "ACM-2026-0412", "PO-1001", "USD", -2, 30, "Net 30", 0, []item{
		{"ACM-100", "Industrial ball bearing 6204", 200, 1250},
		{"ACM-200", "Hydraulic hose 1/2 in x 10 ft", 50, 4800},
		{"ACM-300", "Nitrile safety gloves, box of 100", 30, 2275},
	}},
	{"06-price-within-tolerance", "classic", pacific, pacific, "PCE-55120", "PO-4001", "USD", -8, 30, "Net 30", 0, []item{
		{"PCE-10", "LED driver 40 W", 100, 1818},
		{"PCE-20", "Power cable 2 m", 200, 650},
	}},
	{"07-eur-partial-receipt", "classic", hanse, hanse, "HP-2026-1187", "PO-3001", "EUR", -6, 45, "Net 45", 0, []item{
		{"HPT-1", "Präzisionswelle 20 mm", 60, 14500},
		{"HPT-2", "Kugellager-Set", 60, 8950},
	}},
	{"08-origin-mismatch", "modern", acme, acmeNewYork, "ACM-2026-0433", "PO-1002", "USD", -7, 30, "Net 30", 0, []item{
		{"ACM-500", "Anchor bolt M12", 1000, 85},
	}},
	{"09-payment-unknown", "classic", lakeshore, lakeshore, "LSP-30419", "PO-5001", "USD", -14, 15, "Net 15", 0, []item{
		{"LKP-1", "Corrugated shipping box 40x30x30", 2000, 115},
		{"LKP-2", "Stretch film roll 500 mm", 120, 1400},
	}},
	{"10-payment-declined", "modern", gulf, gulf, "GMP-7781", "PO-6001", "USD", -29, 30, "Net 30", 0, []item{
		{"GMP-1", "Marine fuel filter", 50, 3740},
		{"GMP-2", "Stainless hose clamp", 300, 210},
	}},
}

func (s sample) issueDate(today time.Time) time.Time {
	return today.AddDate(0, 0, s.issueOffset)
}

func (s sample) dueDate(today time.Time) time.Time {
	return s.issueDate(today).AddDate(0, 0, s.netDays)
}
