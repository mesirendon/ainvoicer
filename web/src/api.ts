import type {
  Clock,
  EmailDraft,
  InvoiceDetail,
  InvoiceSummary,
  Meta,
  PaymentSummary,
  PurchaseOrder,
  Sample,
} from './types'

interface ApiErrorBody {
  error: { code: string; message: string; details?: Record<string, unknown> }
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details: Record<string, unknown> = {},
  ) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, init)
  const isJSON = res.headers.get('content-type')?.includes('application/json') ?? false
  const body: unknown = isJSON ? await res.json() : null
  if (!res.ok) {
    const err = (body as ApiErrorBody | null)?.error
    throw new ApiError(res.status, err?.code ?? 'HTTP_ERROR', err?.message ?? res.statusText, err?.details ?? {})
  }
  return body as T
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export const api = {
  meta: () => request<Meta>('/meta'),
  clock: () => request<Clock>('/demo/clock'),
  advanceDays: (days: number) => post<Clock>('/demo/advance-days', { days }),
  resetDemo: () => post<Clock>('/demo/reset'),

  invoices: () => request<InvoiceSummary[]>('/invoices'),
  invoice: (id: string) => request<InvoiceDetail>(`/invoices/${id}`),
  invoiceFileUrl: (id: string) => `/api/invoices/${id}/file`,
  upload: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return request<InvoiceDetail>('/invoices', { method: 'POST', body: form })
  },
  approve: (id: string) => post<InvoiceDetail>(`/invoices/${id}/approve`),
  override: (id: string, reason: string) => post<InvoiceDetail>(`/invoices/${id}/override`, { reason }),
  reject: (id: string, reason: string) => post<InvoiceDetail>(`/invoices/${id}/reject`, { reason }),
  supplierEmail: (id: string) => post<EmailDraft>(`/invoices/${id}/supplier-email`),

  samples: () => request<Sample[]>('/samples'),
  processSample: (name: string) => post<InvoiceDetail>(`/samples/${encodeURIComponent(name)}/process`),

  purchaseOrders: () => request<PurchaseOrder[]>('/purchase-orders'),
  payments: () => request<PaymentSummary[]>('/payments'),
  runDue: () => post<PaymentSummary[]>('/payments/run-due'),
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return `${e.message} (${e.code})`
  if (e instanceof Error) return e.message
  return String(e)
}
