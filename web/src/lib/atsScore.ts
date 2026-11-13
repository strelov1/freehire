// Small readings of an ATS report (internal/candidate/atscheck) shared by the full report
// view and the compact summaries that quote it.

import type { ATSReport } from './types';

const STRONG = 75;
const FAIR = 50;

/** The text colour a 0-100 score reads in: good from 75, middling from 50, poor below. */
export function atsTone(score: number): string {
  if (score >= STRONG) return 'text-primary';
  if (score >= FAIR) return 'text-warning-strong';
  return 'text-destructive';
}

/** The score's band in one word, at the same thresholds as its colour. */
export function atsVerdict(score: number): string {
  if (score >= STRONG) return 'Strong';
  if (score >= FAIR) return 'Fair';
  return 'Needs work';
}

/** The `n` fixes most worth making: the AI review's suggestions when it ran, otherwise the
 *  failed checks and then the warnings, in the report's own order, each named once. */
export function topAtsFixes(report: ATSReport, n = 3): string[] {
  const items = (report.categories ?? []).flatMap((c) => c.items ?? []);
  const texts = report.suggestions?.length
    ? report.suggestions
    : [...items.filter((i) => i.status === 'fail'), ...items.filter((i) => i.status === 'warn')].map((i) => i.text);
  // Two checks can word the same fix alike; it is still one thing to do.
  return [...new Set(texts)].slice(0, n);
}
