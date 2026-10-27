ALTER TABLE invoices ADD COLUMN duplicate_of uuid REFERENCES invoices (id);

DROP INDEX invoices_supplier_number_idx;

CREATE UNIQUE INDEX invoices_supplier_number_idx ON invoices (supplier_id, invoice_number)
    WHERE supplier_id IS NOT NULL AND invoice_number <> '' AND duplicate_of IS NULL;

ALTER TABLE invoice_lines ADD COLUMN po_line_no int;

CREATE INDEX invoices_po_idx ON invoices (po_id);
