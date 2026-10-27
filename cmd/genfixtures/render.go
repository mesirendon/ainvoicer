package main

import (
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

type pdfDoc struct {
	*fpdf.Fpdf
	tr func(string) string
}

func render(p printed, today time.Time, path string) error {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetCreationDate(today)
	pdf.SetModificationDate(today)
	pdf.SetCatalogSort(true)
	pdf.SetTitle(p.invoiceNumber, true)
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()
	d := pdfDoc{Fpdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}

	switch p.layout {
	case "classic":
		d.classic(p)
	case "modern":
		d.modern(p)
	case "factura":
		d.factura(p)
	default:
		return fmt.Errorf("unknown layout %q", p.layout)
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		return fmt.Errorf("write pdf %s: %w", path, err)
	}
	return nil
}

func (d pdfDoc) text(w, h float64, s, border, align string, fill bool) {
	d.CellFormat(w, h, d.tr(s), border, 0, align, fill, 0, "")
}

func (d pdfDoc) line(s string) {
	d.CellFormat(0, 5, d.tr(s), "", 1, "L", false, 0, "")
}

func (d pdfDoc) party(title string, p party) {
	d.SetFont("Helvetica", "B", 9)
	d.line(title)
	d.SetFont("Helvetica", "", 9)
	if p.name != "" {
		d.line(p.name)
	}
	d.line(p.line1)
	d.line(fmt.Sprintf("%s %s, %s", p.city, p.postalCode, p.country))
	if p.taxID != "" {
		d.line("Tax ID: " + p.taxID)
	}
}

func (d pdfDoc) table(headers []string, widths []float64, rows [][]string, aligns []string, headerFill [3]int, zebra bool) {
	d.SetFont("Helvetica", "B", 9)
	d.SetFillColor(headerFill[0], headerFill[1], headerFill[2])
	for i, h := range headers {
		d.text(widths[i], 7, h, "1", aligns[i], true)
	}
	d.Ln(-1)
	d.SetFont("Helvetica", "", 9)
	d.SetFillColor(245, 247, 250)
	for r, row := range rows {
		for i, cell := range row {
			d.text(widths[i], 7, cell, "1", aligns[i], zebra && r%2 == 1)
		}
		d.Ln(-1)
	}
}

func (d pdfDoc) totals(labels, values []string, labelWidth, valueWidth float64) {
	d.Ln(3)
	for i := range labels {
		d.SetX(15 + 186 - labelWidth - valueWidth)
		if i == len(labels)-1 {
			d.SetFont("Helvetica", "B", 10)
		} else {
			d.SetFont("Helvetica", "", 9)
		}
		d.text(labelWidth, 6, labels[i], "", "R", false)
		d.text(valueWidth, 6, values[i], "", "R", false)
		d.Ln(-1)
	}
}

func (d pdfDoc) classic(p printed) {
	d.SetFont("Helvetica", "B", 16)
	d.text(120, 8, p.supplier.name, "", "L", false)
	d.SetFont("Helvetica", "B", 20)
	d.text(0, 8, "INVOICE", "", "R", false)
	d.Ln(12)

	d.SetFont("Helvetica", "", 9)
	meta := [][2]string{
		{"Invoice No.", p.invoiceNumber},
		{"Invoice Date", p.issueDate.Format("January 2, 2006")},
		{"Due Date", p.dueDate.Format("January 2, 2006")},
		{"Customer PO", p.poNumber},
		{"Terms", p.terms},
		{"Currency", p.currency},
	}
	for _, m := range meta {
		d.SetX(130)
		d.SetFont("Helvetica", "B", 9)
		d.text(30, 5, m[0], "", "L", false)
		d.SetFont("Helvetica", "", 9)
		d.text(0, 5, m[1], "", "L", false)
		d.Ln(-1)
	}
	d.SetY(32)
	d.party("From", p.supplier)
	d.Ln(3)
	d.party("Bill To", buyer)
	d.Ln(3)
	d.party("Ship From", p.shipFrom)
	d.Ln(6)

	rows := make([][]string, 0, len(p.lines))
	for _, l := range p.lines {
		rows = append(rows, []string{l.sku, l.description, l.quantity, l.unitPrice, l.lineTotal})
	}
	d.table([]string{"Item", "Description", "Qty", "Unit Price", "Amount"},
		[]float64{25, 81, 20, 30, 30}, rows, []string{"L", "L", "R", "R", "R"}, [3]int{220, 220, 220}, false)
	d.totals([]string{"Subtotal", "Tax", "Total " + p.currency}, []string{p.subtotal, p.tax, p.total}, 40, 30)
	d.Ln(10)
	d.SetFont("Helvetica", "I", 8)
	d.line("Please reference the invoice number with your payment. Thank you for your business.")
}

func (d pdfDoc) modern(p printed) {
	d.SetFillColor(30, 64, 120)
	d.Rect(0, 0, 216, 30, "F")
	d.SetTextColor(255, 255, 255)
	d.SetXY(15, 9)
	d.SetFont("Helvetica", "B", 18)
	d.text(110, 8, p.supplier.name, "", "L", false)
	d.SetFont("Helvetica", "", 11)
	d.text(0, 8, "Invoice "+p.invoiceNumber, "", "R", false)
	d.SetTextColor(0, 0, 0)
	d.SetY(38)

	grid := [][2]string{
		{"ISSUED", p.issueDate.Format(time.DateOnly)},
		{"DUE", p.dueDate.Format(time.DateOnly)},
		{"PURCHASE ORDER", p.poNumber},
		{"PAYMENT TERMS", p.terms},
	}
	for _, g := range grid {
		d.SetFont("Helvetica", "B", 7)
		d.SetTextColor(110, 110, 110)
		d.text(46.5, 4, g[0], "", "L", false)
	}
	d.Ln(-1)
	d.SetTextColor(0, 0, 0)
	for _, g := range grid {
		d.SetFont("Helvetica", "", 10)
		d.text(46.5, 6, g[1], "", "L", false)
	}
	d.Ln(12)

	top := d.GetY()
	d.party("SUPPLIER", p.supplier)
	bottom := d.GetY()
	d.SetY(top)
	d.SetLeftMargin(110)
	d.party("SHIPPED FROM", p.shipFrom)
	d.SetLeftMargin(15)
	d.SetY(max(bottom, d.GetY()) + 4)
	d.party("CUSTOMER", buyer)
	d.Ln(6)

	rows := make([][]string, 0, len(p.lines))
	for _, l := range p.lines {
		rows = append(rows, []string{l.description + " (" + l.sku + ")", l.quantity, l.unitPrice, l.lineTotal})
	}
	d.table([]string{"Description", "Quantity", "Rate", "Amount"},
		[]float64{96, 25, 30, 35}, rows, []string{"L", "R", "R", "R"}, [3]int{200, 215, 235}, true)
	d.totals([]string{"Subtotal", "Sales tax", "Amount due (" + p.currency + ")"}, []string{p.subtotal, p.tax, p.total}, 50, 35)
}

func (d pdfDoc) factura(p printed) {
	d.SetFont("Helvetica", "B", 22)
	d.text(0, 10, "FACTURA", "", "C", false)
	d.Ln(12)
	d.SetFont("Helvetica", "", 10)
	d.text(0, 6, "Folio: "+p.invoiceNumber, "", "C", false)
	d.Ln(10)

	d.SetFont("Helvetica", "B", 10)
	d.line("Emisor")
	d.SetFont("Helvetica", "", 9)
	d.line(p.supplier.name + "  ·  RFC: " + p.supplier.taxID)
	d.line(fmt.Sprintf("%s, %s, C.P. %s, México", p.supplier.line1, p.supplier.city, p.supplier.postalCode))
	d.Ln(2)
	d.SetFont("Helvetica", "B", 10)
	d.line("Receptor")
	d.SetFont("Helvetica", "", 9)
	d.line(buyer.name + "  ·  " + buyer.taxID)
	d.line(fmt.Sprintf("%s, %s, Colombia", buyer.line1, buyer.city))
	d.Ln(2)
	d.SetFont("Helvetica", "B", 10)
	d.line("Lugar de expedición / Enviado desde")
	d.SetFont("Helvetica", "", 9)
	d.line(fmt.Sprintf("%s, %s, C.P. %s, México", p.shipFrom.line1, p.shipFrom.city, p.shipFrom.postalCode))
	d.Ln(4)

	info := [][2]string{
		{"Fecha de emisión", p.issueDate.Format("02/01/2006")},
		{"Fecha de vencimiento", p.dueDate.Format("02/01/2006")},
		{"Orden de compra", p.poNumber},
		{"Condiciones de pago", p.terms},
		{"Moneda", p.currency + " - Peso mexicano"},
	}
	for _, i := range info {
		d.SetFont("Helvetica", "B", 9)
		d.text(45, 5, i[0]+":", "", "L", false)
		d.SetFont("Helvetica", "", 9)
		d.line(i[1])
	}
	d.Ln(4)

	rows := make([][]string, 0, len(p.lines))
	for _, l := range p.lines {
		rows = append(rows, []string{l.quantity, l.sku, l.description, l.unitPrice, l.lineTotal})
	}
	d.table([]string{"Cantidad", "Clave", "Descripción", "Precio unitario", "Importe"},
		[]float64{22, 24, 76, 32, 32}, rows, []string{"R", "L", "L", "R", "R"}, [3]int{235, 225, 205}, false)
	d.totals([]string{"Subtotal", "IVA 16%", "Total"}, []string{p.subtotal, p.tax, p.total}, 40, 32)
	d.Ln(8)
	d.SetFont("Helvetica", "I", 8)
	d.line("Pronto pago: 2% de descuento si se paga dentro de los 10 días naturales siguientes a la emisión.")
}
