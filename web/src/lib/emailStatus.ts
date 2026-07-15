// Presentation for the classified email status signal (mailclassify vocabulary):
// a short human label and an outline-badge colour class. `other` renders nothing.

export const STATUS_LABELS: Record<string, string> = {
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

export const STATUS_CLASSES: Record<string, string> = {
  acknowledgement: 'border-border text-muted-foreground',
  screening: 'border-blue-400/40 text-blue-600 dark:text-blue-400',
  interview_invitation: 'border-emerald-400/50 text-emerald-600 dark:text-emerald-400',
  assessment: 'border-indigo-400/40 text-indigo-600 dark:text-indigo-400',
  offer: 'border-emerald-500/60 font-semibold text-emerald-700 dark:text-emerald-300',
  rejection: 'border-destructive/40 text-destructive',
  info_request: 'border-amber-400/50 text-amber-600 dark:text-amber-400',
  incomplete_application: 'border-orange-400/50 text-orange-600 dark:text-orange-400',
  other: '',
};

/** The label for a status signal, or '' when unknown/other (renders nothing). */
export function statusLabel(signal?: string): string {
  return signal ? (STATUS_LABELS[signal] ?? '') : '';
}

/** The badge colour class for a status signal. */
export function statusClass(signal?: string): string {
  return signal ? (STATUS_CLASSES[signal] ?? '') : '';
}
