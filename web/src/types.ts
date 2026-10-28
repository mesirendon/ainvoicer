export type InvoiceStatus =
  | 'RECEIVED'
  | 'EXTRACTION_FAILED'
  | 'MATCHED'
  | 'MATCHED_WITH_WARNINGS'
  | 'BLOCKED'
  | 'APPROVED'
  | 'REJECTED'
  | 'PAYMENT_SCHEDULED'
  | 'PAID'
  | 'PAYMENT_FAILED'
  | 'PAYMENT_UNKNOWN'

export type PaymentStatus = 'SCHEDULED' | 'PROCESSING' | 'PAID' | 'FAILED' | 'UNKNOWN'

export interface Meta {
  fixtureMode: boolean
  model: string
  buyerName: string
  buyerCountry: string
  fundingCurrency: string
}

export interface Clock {
  today: string
  offsetDays: number
}

export interface Sample {
  name: string
  fileName: string
}

export interface InvoiceSummary {
  id: string
  fileName: string
  supplierName: string
  invoiceNumber: string
  currency: string
  totalMinor: number
  status: InvoiceStatus
  createdAt: string
}

export interface InvoiceLine {
  lineNo: number
  poLineNo: number | null
  sku: string
  description: string
  quantity: number
  unitPriceMinor: number
  lineTotalMinor: number
}

export interface Finding {
  id: number
  lineNo: number | null
  code: string
  severity: 'WARNING' | 'BLOCK'
  expected: string
  actual: string
  message: string
}

export interface POLine {
  lineNo: number
  sku: string
  description: string
  quantity: number
  unitPriceMinor: number
}

export interface ReceiptLine {
  sku: string
  quantity: number
}

export interface Receipt {
  id: string
  warehouseName: string
  warehouseCity: string
  warehouseCountry: string
  receivedOn: string
  lines: ReceiptLine[]
}

export interface PurchaseOrder {
  id: string
  number: string
  supplierId: string
  supplierName: string
  currency: string
  status: string
  lines: POLine[]
  receipts: Receipt[]
}

export interface Payment {
  id: string
  status: PaymentStatus
  amountMinor: number
  discountMinor: number
  currency: string
  scheduledFor: string
  fundingCurrency: string
  fxRateE8: number
  fxSpreadBps: number
  fundingAmountMinor: number
  processorPaymentId: string
  error: string
  updatedAt: string
}

export interface AuditEntry {
  actor: string
  action: string
  detail: Record<string, unknown>
  createdAt: string
}

export interface InvoiceDetail {
  id: string
  fileName: string
  status: InvoiceStatus
  supplierId: string
  supplierName: string
  supplierCountry: string
  invoiceNumber: string
  poNumber: string
  currency: string
  subtotalMinor: number
  taxMinor: number
  totalMinor: number
  issueDate: string
  dueDate: string
  shipFromCity: string
  shipFromCountry: string
  duplicateOf: string
  extractionError: string
  createdAt: string
  updatedAt: string
  lines: InvoiceLine[]
  findings: Finding[]
  purchaseOrder: PurchaseOrder | null
  payment: Payment | null
  audit: AuditEntry[]
}

export interface EmailDraft {
  subject: string
  body: string
  language: 'en' | 'es'
  generatedBy: 'claude' | 'template'
}

export interface PaymentSummary extends Payment {
  invoiceId: string
  invoiceNumber: string
  supplierName: string
  destinationLast4: string
}
