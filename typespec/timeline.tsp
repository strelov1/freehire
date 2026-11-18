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

// The caller's events between `from` and `to` inclusive, oldest first.
/**
 * The application-event ledger as a dated series, oldest first: applications sent,
 * employer replies, follow-ups and stage changes. `occurred_at` is an instant, not a day
 * — which day it falls on depends on your timezone, so group it client-side. `observed`
 * says whether the date came from a source other than you: mail-derived events carry a
 * date the employer set, while a stage you set yourself is dated from when you recorded
 * it. `email_id` and `email_subject` are present only while the message exists; deleting
 * it hides the content and leaves the event standing. Both bounds are required and may
 * span at most 366 days.
 */
@summary("What happened to your applications over a date range.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: 7,
        kind: "employer_reply",
        signal: "interview_invitation",
        source: "mail_gmail",
        observed: true,
        occurred_at: utcDateTime.fromISO("2026-08-13T09:41:00Z"),
        company_slug: "derq",
        role_title: "Senior Go Engineer",
        application_id: 31,
        job_slug: "senior-go-engineer-derq-1a2b",
        email_id: 42,
        email_subject: "Invitation to interview",
      },
      #{
        id: 8,
        kind: "stage_set",
        signal: "screening",
        source: "user",
        observed: false,
        occurred_at: utcDateTime.fromISO("2026-08-13T21:15:00Z"),
        company_slug: "linear",
        application_id: 33,
      },
    ],
    meta: #{
      from: utcDateTime.fromISO("2026-08-01T00:00:00Z"),
      to: utcDateTime.fromISO("2026-08-31T23:59:59Z"),
      count: 2,
    },
  },
})
@tag("Tracking")
@route("/me/timeline")
@get
op getTimeline(/** Lower bound, inclusive. Use `Z`, or percent-encode a numeric offset — a bare `+` decodes as a space in a query string and will be rejected. (e.g. `2026-08-01T00:00:00Z`) */ @query from?: string, /** Upper bound, inclusive. (e.g. `2026-08-31T23:59:59Z`) */ @query to?: string): {
  data: TimelineEvent[];
  meta: TimelineMeta;
} | Problem;

// The caller's arranged meetings whose start falls in the range.
/**
 * The calendar’s second layer beside `/me/timeline`: a meeting is arranged and can move
 * or be cancelled, unlike a ledger event which happened and cannot change. `status` is
 * `suggested`, `confirmed`, or `cancelled` — a cancelled meeting is still served, not
 * withheld. Both bounds are required, RFC3339.
 */
@summary("Arranged meetings whose start falls in the date range.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: 3,
        application_id: 31,
        starts_at: utcDateTime.fromISO("2026-08-20T13:00:00Z"),
        ends_at: utcDateTime.fromISO("2026-08-20T13:30:00Z"),
        title: "Interview with Acme",
        join_url: "https://meet.example.com/abc",
        status: "confirmed",
        company_slug: "acme",
        role_title: "Senior Go Engineer",
        job_slug: "senior-go-engineer-acme-1a2b",
      },
    ],
    meta: #{
      from: utcDateTime.fromISO("2026-08-01T00:00:00Z"),
      to: utcDateTime.fromISO("2026-08-31T23:59:59Z"),
      count: 1,
    },
  },
})
@tag("Tracking")
@route("/me/interviews")
@get
op getInterviews(/** Lower bound, inclusive. (e.g. `2026-08-01T00:00:00Z`) */ @query from?: string, /** Upper bound, inclusive. (e.g. `2026-08-31T23:59:59Z`) */ @query to?: string): {
  data: ScheduledInterview[];
  meta: TimelineMeta;
} | Problem;
