CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE suppliers (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    tax_id text NOT NULL DEFAULT '',
    country text NOT NULL,
    currency text NOT NULL,
    bank_account text NOT NULL,
    payment_net_days int NOT NULL CHECK (payment_net_days >= 0),
    early_discount_bps int NOT NULL DEFAULT 0 CHECK (early_discount_bps >= 0 AND early_discount_bps <= 10000),
    early_discount_days int NOT NULL DEFAULT 0 CHECK (early_discount_days >= 0)
);

CREATE TABLE warehouses (
    id uuid PRIMARY KEY,
    supplier_id uuid NOT NULL REFERENCES suppliers (id),
    name text NOT NULL,
    city text NOT NULL,
    country text NOT NULL,
    location geography(Point, 4326) NOT NULL
);

CREATE INDEX warehouses_location_idx ON warehouses USING gist (location);

CREATE TABLE purchase_orders (
    id uuid PRIMARY KEY,
    po_number text NOT NULL UNIQUE,
    supplier_id uuid NOT NULL REFERENCES suppliers (id),
    currency text NOT NULL,
    status text NOT NULL CHECK (status IN ('OPEN', 'CLOSED')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE po_lines (
    po_id uuid NOT NULL REFERENCES purchase_orders (id),
    line_no int NOT NULL,
    sku text NOT NULL,
    description text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor > 0),
    PRIMARY KEY (po_id, line_no)
);

CREATE TABLE goods_receipts (
    id uuid PRIMARY KEY,
    po_id uuid NOT NULL REFERENCES purchase_orders (id),
    warehouse_id uuid NOT NULL REFERENCES warehouses (id),
    received_on date NOT NULL
);

CREATE TABLE receipt_lines (
    receipt_id uuid NOT NULL REFERENCES goods_receipts (id),
    sku text NOT NULL,
    quantity_received bigint NOT NULL CHECK (quantity_received > 0),
    PRIMARY KEY (receipt_id, sku)
);

CREATE TABLE invoices (
    id uuid PRIMARY KEY,
    file_name text NOT NULL,
    file_sha256 text NOT NULL UNIQUE,
    file bytea NOT NULL,
    supplier_id uuid REFERENCES suppliers (id),
    po_id uuid REFERENCES purchase_orders (id),
    invoice_number text NOT NULL DEFAULT '',
    currency text NOT NULL DEFAULT '',
    subtotal_minor bigint NOT NULL DEFAULT 0,
    tax_minor bigint NOT NULL DEFAULT 0,
    total_minor bigint NOT NULL DEFAULT 0,
    issue_date date,
    due_date date,
    ship_from_city text NOT NULL DEFAULT '',
    ship_from_country text NOT NULL DEFAULT '',
    ship_from_location geography(Point, 4326),
    extraction jsonb,
    extraction_error text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN (
        'RECEIVED', 'EXTRACTION_FAILED', 'MATCHED', 'MATCHED_WITH_WARNINGS', 'BLOCKED',
        'APPROVED', 'REJECTED', 'PAYMENT_SCHEDULED', 'PAID', 'PAYMENT_FAILED', 'PAYMENT_UNKNOWN'
    )),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX invoices_supplier_number_idx ON invoices (supplier_id, invoice_number) WHERE supplier_id IS NOT NULL AND invoice_number <> '';

CREATE TABLE invoice_lines (
    invoice_id uuid NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    line_no int NOT NULL,
    sku text NOT NULL DEFAULT '',
    description text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    line_total_minor bigint NOT NULL CHECK (line_total_minor >= 0),
    PRIMARY KEY (invoice_id, line_no)
);

CREATE TABLE findings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id uuid NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    line_no int,
    code text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('WARNING', 'BLOCK')),
    expected text NOT NULL DEFAULT '',
    actual text NOT NULL DEFAULT '',
    message text NOT NULL
);

CREATE INDEX findings_invoice_idx ON findings (invoice_id);

CREATE TABLE payments (
    id uuid PRIMARY KEY,
    invoice_id uuid NOT NULL UNIQUE REFERENCES invoices (id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('SCHEDULED', 'PROCESSING', 'PAID', 'FAILED', 'UNKNOWN')),
    amount_minor bigint NOT NULL CHECK (amount_minor > 0),
    currency text NOT NULL,
    discount_minor bigint NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    scheduled_for date NOT NULL,
    funding_currency text NOT NULL,
    fx_rate_e8 bigint NOT NULL CHECK (fx_rate_e8 > 0),
    fx_spread_bps int NOT NULL CHECK (fx_spread_bps >= 0),
    funding_amount_minor bigint NOT NULL CHECK (funding_amount_minor > 0),
    processor_payment_id text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX payments_due_idx ON payments (status, scheduled_for);

CREATE TABLE fx_rates (
    base text NOT NULL,
    quote text NOT NULL,
    rate_e8 bigint NOT NULL CHECK (rate_e8 > 0),
    spread_bps int NOT NULL CHECK (spread_bps >= 0),
    PRIMARY KEY (base, quote)
);

CREATE TABLE audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id uuid NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    actor text NOT NULL,
    action text NOT NULL,
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_invoice_idx ON audit_log (invoice_id, created_at);

CREATE TABLE demo_clock (
    id int PRIMARY KEY CHECK (id = 1),
    offset_days int NOT NULL DEFAULT 0
);

INSERT INTO demo_clock (id, offset_days) VALUES (1, 0);
