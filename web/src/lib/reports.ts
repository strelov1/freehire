// The report-reason vocabulary, mirroring the backend's internal/report reasons.
// One source of truth for both the report dialog (the reason picker) and the
// moderator queue (rendering a stored reason). Kept beside the API client rather
// than generated, since these reasons live in the report package, not the
// enrichment contract that cmd/gen-contracts emits.
import type { ReportReason } from './types';

interface ReasonOption {
  value: ReportReason;
  /** Short label for the picker and the queue. */
  label: string;
  /** One-line help shown under the label in the picker. */
  hint: string;
}

/** The reasons in the order they appear in the picker (mirrors the mockup). */
export const reportReasons: ReasonOption[] = [
  { value: 'no_response', label: 'No response', hint: 'Applied but never heard back' },
  { value: 'not_relevant', label: 'No longer relevant', hint: 'Position filled or expired' },
  { value: 'spam', label: 'Spam or not a job', hint: 'An ad or not a real vacancy' },
  { value: 'fraud', label: 'Fraud', hint: 'A scam or asks for payment' },
  { value: 'other', label: 'Other', hint: 'Something else is wrong' },
];

/** Label for a stored reason value (used by the moderator queue). */
export function reportReasonLabel(reason: ReportReason): string {
  return reportReasons.find((r) => r.value === reason)?.label ?? reason;
}
