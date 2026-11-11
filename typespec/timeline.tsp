import "@typespec/http";

using Http;

namespace Freehire;

// The caller's application ledger over a dated range, and the meetings arranged in it.
// With a session or an API key. Both bounds are required RFC3339 instants; they are read
// leniently (a missing or unreadable one is a 400 with its own message), so they are
// declared as plain strings.

/** One ledger event. An event happened and cannot change. */
model TimelineEvent {
  id: int64;
  kind: string;
  signal?: string;
  source: string;

  /** The server's verdict on whether somebody other than the candidate set the date. */
  observed: boolean;

  /** An instant, not a day: which day it falls on depends on the reader's clock. */
  occurred_at: utcDateTime;

  company_slug: string;
  role_title?: string;
  application_id?: int64;
  job_slug?: string;
  email_id?: int64;
  email_subject?: string;
}

/** One arranged meeting. Unlike an event it can move or be called off. */
model ScheduledInterview {
  id: int64;
  application_id: int64;
  starts_at: utcDateTime;

  /** Absent for an all-day entry. */
  ends_at?: utcDateTime;

  title?: string;
  join_url?: string;

  /** suggested, confirmed or cancelled. A cancelled meeting is served. */
  status: string;

  company_slug: string;
  role_title?: string;
  job_slug?: string;
}

model TimelineMeta {
  from: utcDateTime;
  to: utcDateTime;
  count: int64;
}

/** The caller's events between `from` and `to` inclusive, oldest first. */
@tag("Tracking")
@route("/me/timeline")
@get
op getTimeline(@query from?: string, @query to?: string): {
  data: TimelineEvent[];
  meta: TimelineMeta;
} | Problem;

/** The caller's arranged meetings whose start falls in the range. */
@tag("Tracking")
@route("/me/interviews")
@get
op getInterviews(@query from?: string, @query to?: string): {
  data: ScheduledInterview[];
  meta: TimelineMeta;
} | Problem;
