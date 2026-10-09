// How a ledger event reads: its label and the tone it is drawn in.
//
// One definition for the two surfaces that render the same events — the calendar, which paints
// a month, and the application panel, which lists one application's history. These labels lived
// inside TrackingCalendar.svelte until the panel needed them; copying them would have meant the
// same event captioned two ways on two screens.
//
// `KIND_LABEL` is checked against the generated vocabulary (`APPLICATION_EVENT_KINDS`), so a
// kind added in Go and forgotten here fails a test rather than falling through to the fallback,
// which reads plausibly while dropping what the label was for.

import type { TimelineEvent } from './types';
import { format, t } from './i18n/t';
import type { Locale } from './locale';
import { messages } from './events.messages';

/** Sentence-case an unknown kind so a new one reads as words, not as a column name. */
function humanKind(kind: string): string {
  const words = kind.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export const KIND_LABEL: Record<string, (e: TimelineEvent, locale: Locale) => string> = {
  applied: (_e, locale) => t(messages, locale).applied,
  employer_reply: (e, locale) => {
    const s = t(messages, locale);
    return e.signal ? format(s.employerRepliedWithSignal, { signal: e.signal.replace(/_/g, ' ') }) : s.employerReplied;
  },
  follow_up_sent: (_e, locale) => t(messages, locale).followedUp,
  stage_set: (e, locale) => {
    const s = t(messages, locale);
    return e.signal ? format(s.movedTo, { signal: e.signal }) : s.stageChanged;
  },
  interview_scheduled: (_e, locale) => t(messages, locale).interviewScheduled,
};

/** What happened, in a phrase. A kind from a server newer than this build is sentence-cased
 *  rather than left blank. */
export function eventLabel(e: TimelineEvent, locale: Locale = 'en'): string {
  const fn = KIND_LABEL[e.kind];
  return fn ? fn(e, locale) : humanKind(e.kind);
}

// Design tokens, not palette utilities: `pnpm check:tokens` counts raw colours per file, and
// only four of these read as distinct — `primary` and `secondary-foreground` hold the same
// value in both themes, so reaching for the second to separate two kinds collapses them.
const KIND_TONE: Record<string, string> = {
  applied: 'text-primary',
  employer_reply: 'text-brand-strong',
  follow_up_sent: 'text-warning-strong',
  stage_set: 'text-muted-foreground',
  interview_scheduled: 'text-brand-strong',
};

/** The tone a kind is drawn in; quiet for anything unrecognised. */
export function eventTone(kind: string): string {
  return KIND_TONE[kind] ?? 'text-muted-foreground';
}
