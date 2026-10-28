#!/usr/bin/env bash
set -euo pipefail

API="${API:-http://localhost:3000/api}"
cd "$(dirname "$0")/.."

for tool in curl jq; do
  command -v "$tool" >/dev/null || { echo "demo.sh needs $tool"; exit 1; }
done

MONEY='def money: tostring | (if length < 3 then ("00" + .)[-3:] else . end) | .[:-2] + "." + .[-2:];'

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
post() { curl -sS -X POST -H 'Content-Type: application/json' ${2:+-d "$2"} "$API$1"; }
invoice_id() { curl -sS "$API/invoices" | jq -r --arg n "$1" --arg s "$2" '.[] | select(.invoiceNumber == $n and .status == $s) | .id' | head -1; }
show_payments() {
  curl -sS "$API/payments" | jq -r "$MONEY"'.[] | [.invoiceNumber, .supplierName, "\(.amountMinor | money) \(.currency)", .scheduledFor, .status, (.error | .[0:60])] | @tsv' | column -t -s $'\t'
}

step "API health"
curl -sS -f "$API/health" && echo

step "Race check: 10 parallel uploads of the same file must create exactly one invoice"
post /demo/reset >/dev/null
for _ in $(seq 10); do
  curl -sS -o /dev/null -w '%{http_code}\n' -F "file=@samples/01-clean-usd.pdf" "$API/invoices" &
done | sort | uniq -c
wait
count=$(curl -sS "$API/invoices" | jq length)
if [ "$count" != "1" ]; then echo "FAIL: $count invoices"; exit 1; fi
echo "OK: exactly one invoice"

step "Reset the demo and process every sample"
post /demo/reset | jq -c .
for name in $(curl -sS "$API/samples" | jq -r '.[].name'); do
  post "/samples/$name/process" | jq -r --arg n "$name" '[$n, .status, ([.findings[].code] | join(", "))] | @tsv'
done | column -t -s $'\t'

step "Human decisions"
echo "approve the 1% price variance (Pacific Coast, bank account 1111):"
post "/invoices/$(invoice_id PCE-55120 MATCHED_WITH_WARNINGS)/approve" | jq -r '"  \(.status), pays \(.payment.scheduledFor)"'
echo "approve the origin warning (ships from New York):"
post "/invoices/$(invoice_id ACM-2026-0433 MATCHED_WITH_WARNINGS)/approve" | jq -r '"  \(.status), pays \(.payment.scheduledFor)"'
echo "override the quantity block with a reason:"
post "/invoices/$(invoice_id ACM-2026-0432 BLOCKED)/override" '{"reason":"Remaining 30 valves arrived; receipt not yet posted in the ERP"}' | jq -r '"  \(.status)"'
duplicate=$(invoice_id ACM-2026-0412 BLOCKED)
echo "try to override the duplicate (must be refused):"
post "/invoices/$duplicate/override" '{"reason":"pay it anyway"}' | jq -r '"  \(.error.code): \(.error.message)"'
echo "reject the duplicate and the 8% price variance:"
post "/invoices/$duplicate/reject" '{"reason":"Duplicate of ACM-2026-0412"}' | jq -r '"  \(.status)"'
post "/invoices/$(invoice_id ACM-2026-0431 BLOCKED)/reject" '{"reason":"Price increase not agreed"}' | jq -r '"  \(.status)"'

step "Scheduled payments and FX quotes"
show_payments
curl -sS "$API/invoices/$(invoice_id TDN-F-00871 PAYMENT_SCHEDULED)" |
  jq -r "$MONEY"'.payment | "early discount: supplier receives \(.amountMinor | money) \(.currency) (saved \(.discountMinor | money)), you pay \(.fundingAmountMinor | money) \(.fundingCurrency)"'

step "Fast-forward 1 day and run due payments (0000 is declined, 2222 times out and becomes UNKNOWN)"
post /demo/advance-days '{"days":1}' | jq -c .
post /payments/run-due >/dev/null
show_payments

step "Wait for the reconciler to resolve the UNKNOWN payment"
for _ in $(seq 24); do
  status=$(curl -sS "$API/payments" | jq -r '.[] | select(.invoiceNumber == "LSP-30419") | .status')
  echo "  LSP-30419: $status"
  [ "$status" = "FAILED" ] || [ "$status" = "PAID" ] && break
  sleep 5
done

step "Fast-forward 45 days and run due payments (1111 times out, then the lookup finds it paid)"
post /demo/advance-days '{"days":45}' | jq -c .
post /payments/run-due >/dev/null
show_payments

step "Final invoice statuses"
curl -sS "$API/invoices" | jq -r '.[] | [.invoiceNumber, .supplierName, .status] | @tsv' | column -t -s $'\t'
