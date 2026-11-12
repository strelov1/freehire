import type { components } from '#lib/generated/api.js';

/** A work-history/CV period boundary: a year, and optionally a month (1-12; absent means
 *  the CV gave no month). Named PeriodDate, not Date, so it never shadows the language's
 *  own global `Date` wherever this file's types are imported — mirrors the Go type
 *  (internal/candidate/perioddate.PeriodDate) this is the wire shape of. */
export type PeriodDate = components['schemas']['PeriodDate'];

/** One place where evidence was produced: a job or a project. Start/end are a structured
 *  PeriodDate, matching the Go domain type — no parsing needed on this side either.
 *  Jobs expose `company`; projects expose the place label as `name` (same storage column). */
export type ExperienceEmployment = {
  id: string;
  kind: 'job' | 'project';
  /** Employer name — present for `kind: 'job'`. */
  company?: string;
  /** Project title — present for `kind: 'project'` (not a company). */
  name?: string;
  role?: string;
  location?: string;
  start?: PeriodDate;
  end?: PeriodDate;
  current?: boolean;
  summary?: string;
  /** Portfolio URL — typically set on projects. */
  link?: string;
  stack?: string[];
};

/** How the bank came to hold an achievement. Only the first three may be written into a
 *  CV: they are the candidate speaking. `agent_inferred` is the assistant's own reading,
 *  kept so it can be asked about and barred from the page until confirmed. */
export type ExperienceProvenance = 'cv_import' | 'stated_in_chat' | 'manual' | 'agent_inferred';

/** One piece of evidence, at the grain of a CV bullet.
 *
 *  `needs_context`, `needs_metrics`, and `cluster_id` arrive only on the list
 *  projection — they are computed at read time and must not be POSTed back. */
export type ExperienceAtom = {
  id: string;
  employment_id?: string;
  claim: string;
  context?: string;
  metrics?: string[];
  skills?: string[];
  provenance: ExperienceProvenance;
  source_ref?: string;
  needs_context?: boolean;
  needs_metrics?: boolean;
  /** Shared id of a soft-duplicate group (first member), when the list clustered this atom. */
  cluster_id?: string;
};

export type ExperienceEmploymentWithAtoms = ExperienceEmployment & { atoms: ExperienceAtom[] };

/** The bank as its owner sees it. `unplaced` carries achievements belonging to no role —
 *  usually the ones volunteered in conversation, so they are shown rather than hidden. */
export type ExperienceBank = {
  employments: ExperienceEmploymentWithAtoms[];
  unplaced: ExperienceAtom[];
};

/** One thing that happened to one application, as GET /me/timeline serves it.
 *
 *  `kind` and `signal` are values from the server's vocabularies rather than a closed
 *  union: `application_events` splits the two precisely so a growing vocabulary is not a
 *  change to a table that must not change, and a union here would make every addition a
 *  change to the SPA as well. Render an unrecognised kind, do not drop it.
 *
 *  `occurred_at` is an instant, not a day. Which day it falls on depends on the reader's
 *  clock — see calendarModel.
 *
 *  `observed` is the server's verdict on the source: mail-derived events carry a date
 *  somebody other than the candidate set, a stage they moved themselves carries the date
 *  they got around to recording it. Never re-derive this from `source` here; the rule
 *  lives in Go and a second copy would drift from it. */
export type TimelineEvent = components['schemas']['TimelineEvent'];

/** One interview arranged for an application, as GET /me/interviews serves it.
 *
 *  Deliberately a different type from [[TimelineEvent]], and not a variant of it: an
 *  event happened and cannot change, a meeting is arranged and can be moved or called
 *  off. One shape for both would invite the calendar to draw them with one meaning.
 *
 *  `starts_at` is an instant like `occurred_at`, and belongs to the reader's day for the
 *  same reason — see calendarModel.
 *
 *  `status` is `suggested` when only the meeting's title named the employer, `confirmed`
 *  when the invitation's own identifier attached it, and `cancelled` when the organiser
 *  called it off. A cancelled meeting is still shown: an interview that vanished from a
 *  Thursday cannot be told apart from a calendar that failed to load. */
export type ScheduledInterview = components['schemas']['ScheduledInterview'];

/** One row the suggestions endpoint offers. It completes a PHRASE, so `parts` can
 *  name more than one thing at once — "Senior Software Engineer Google" carries the
 *  role and the company, and applying only one of them discards what was typed. */
export type ApiSuggestion = components['schemas']['Suggestion'];

export type ApiSuggestionPart = components['schemas']['SuggestionPart'];
