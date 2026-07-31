// Presentation for the classified email status signal: a short human label and an
// outline-badge colour class. `other` renders nothing.
//
// Both maps are keyed by the GENERATED vocabulary, not by `string`. Go owns which
// signals exist (`mailclassify.SignalValues` → `EMAIL_STATUS_SIGNAL_VALUES` via
// `make gen-contracts`); before that, a signal added there rendered as a blank chip
// here with every test still green. `Record<EmailStatusSignal, …>` turns the same
// omission into a `pnpm check` failure.

import type { EmailStatusSignal } from '$lib/generated/contracts';

export const STATUS_LABELS: Record<EmailStatusSignal, string> = {
  acknowledgement: 'Received',
  screening: 'Screening',
  interview_invitation: 'Interview',
  assessment: 'Assessment',
  offer: 'Offer',
  rejection: 'Rejected',
  info_request: 'Info requested',
  incomplete_application: 'Incomplete',
  other: '',
};

export const STATUS_CLASSES: Record<EmailStatusSignal, string> = {
  acknowledgement: 'border-border text-muted-foreground',
  screening: 'border-blue-400/40 text-blue-600 dark:text-blue-400',
  interview_invitation: 'border-emerald-400/50 text-emerald-600 dark:text-emerald-400',
  assessment: 'border-indigo-400/40 text-indigo-600 dark:text-indigo-400',
  offer: 'border-emerald-500/60 font-semibold text-emerald-700 dark:text-emerald-300',
  rejection: 'border-destructive/40 text-destructive',
  info_request: 'border-warning/50 text-warning-strong',
  incomplete_application: 'border-orange-400/50 text-orange-600 dark:text-orange-400',
  other: '',
};

/**
 * The label for a status signal, or '' when the signal is unknown or `other`
 * (both render nothing). The argument stays `string`: it arrives from the API, and
 * a server ahead of this build may name a signal this one has never heard of.
 */
export function statusLabel(signal?: string): string {
  return signal ? (STATUS_LABELS[signal as EmailStatusSignal] ?? '') : '';
}

/** The badge colour class for a status signal. */
export function statusClass(signal?: string): string {
  return signal ? (STATUS_CLASSES[signal as EmailStatusSignal] ?? '') : '';
}
