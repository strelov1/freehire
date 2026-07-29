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

/** What a moderator decided: close the vacancy too, resolve and leave it up, or dismiss. */
export type DecisionKind = 'close' | 'resolve' | 'dismiss';

/** The button label for each decision. */
export function decisionLabel(kind: DecisionKind): string {
  switch (kind) {
    case 'close':
      return 'Resolve and close job';
    case 'resolve':
      return 'Resolve, keep open';
    case 'dismiss':
      return 'Dismiss';
  }
}

/** What the decision form asks the moderator to write, per decision. */
export function decisionNotePrompt(kind: DecisionKind): string {
  return kind === 'dismiss'
    ? 'Why nothing changed — the reporter reads this'
    : 'What you did about it — the reporter reads this';
}

interface DecisionResult {
  kind: DecisionKind;
  /** Whether the moderator asked for the reporter to be told. */
  notifyRequested: boolean;
  /** Whether the server reports the notice was actually delivered. */
  notified: boolean;
}

/**
 * The message to show after a decision, or null when there is nothing to say.
 *
 * A decision always stands, even when its notice fails — so the only thing worth
 * interrupting the moderator for is the gap between the two: they asked to tell the
 * reporter and the reporter was not told.
 */
export function decisionOutcome({ notifyRequested, notified }: DecisionResult): string | null {
  if (!notifyRequested || notified) return null;
  return 'The decision was recorded, but the email to the reporter did not go out. Follow up by hand if it matters.';
}
