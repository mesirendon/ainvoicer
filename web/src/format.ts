const amountFormat = new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const rateFormat = new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 })
const dateTimeFormat = new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeStyle: 'short' })

export function money(minor: number, currency: string): string {
  return `${amountFormat.format(minor / 100)} ${currency}`
}

export function rate(rateE8: number): string {
  return rateFormat.format(rateE8 / 100_000_000)
}

export function percent(bps: number): string {
  return `${amountFormat.format(bps / 100)}%`
}

export function dateTime(iso: string): string {
  return dateTimeFormat.format(new Date(iso))
}

export function humanize(code: string): string {
  return code.toLowerCase().replaceAll('_', ' ')
}

const tones: Record<string, string> = {
  MATCHED: 'green',
  APPROVED: 'green',
  MATCHED_WITH_WARNINGS: 'amber',
  PAYMENT_UNKNOWN: 'amber',
  UNKNOWN: 'amber',
  BLOCKED: 'red',
  EXTRACTION_FAILED: 'red',
  PAYMENT_FAILED: 'red',
  FAILED: 'red',
  PAYMENT_SCHEDULED: 'blue',
  SCHEDULED: 'blue',
  PROCESSING: 'blue',
  RECEIVED: 'blue',
  PAID: 'grey',
  REJECTED: 'dark',
}

export function tone(status: string): string {
  return tones[status] ?? 'grey'
}
