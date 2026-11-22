import "@typespec/http";

using Http;

namespace Freehire;

// The caller's job tracking board: save, dismiss, apply, stage and notes, the listing that
// joins them with the jobs, the slug sets the web app dims its cards with, the pipeline
// snapshot and the account's notification rule. With a session or an API key, except the
// notification rule, which is the session cookie only. The swipe deck stays hand-written:
// it reads the whole search vocabulary.

/**
 * The caller's interaction with one job. Every timestamp, the stage and the notes are null
 * until the job is viewed, saved, applied to, dismissed or tracked.
 */
model Interaction {
  job_id: int64;
  viewed_at: utcDateTime | null;
  saved_at: utcDateTime | null;
  applied_at: utcDateTime | null;
  dismissed_at: utcDateTime | null;
  stage: string | null;
  notes: string | null;
}

/** The interaction after an apply, plus the application it wrote. */
model AppliedInteraction {
  ...Interaction;
  /** The tracking-listing row id of the application (`a<id>`); pass it to `DELETE /me/applications/{id}` to undo. */
  application_id: string;
}

model ApplyJob {
  /** The day the application was sent, `2026-07-29`. Absent, null or empty means now. */
  applied_on?: string | null;
}

/** At least one of the two is required. A field left out or null keeps the stored value. */
model TrackJob {
  /** Application stage from the vocabulary above. (e.g. `interview`) */
  stage?: string | null;
  /** Free-text notes. */
  notes?: string | null;
}

/** Where an application sits against the silence thresholds of its stage (internal/job/silence). */
enum SilenceState {
  active,
  silent,
  unconfirmed,
}

/** The candidate-facing status of a live auto-apply attempt (autoapply.Status). */
enum AutoApplyReviewStatus {
  tailoring,
  pending_review,
  approved,
  blocked,
  declined,
  failed,

  /** The tailoring run gave up without a CV; `failed` is a submission that gave up with one. */
  tailor_failed,
}

/** One resolved question and answer in an auto-apply answer preview. */
model AutoApplyPreviewField {
  label: string;
  value: string;
}

/** A required question the preview has no answer for yet. */
model AutoApplyPreviewPending {
  label: string;

  /** Whether the real submission will fill it in on its own. */
  will_draft_at_submission: boolean;
}

/** What an unattended submission would send, computed once before review. */
model AutoApplyResolvedPreview {
  fields: AutoApplyPreviewField[];
  pending?: AutoApplyPreviewPending[];
}

/** A question the attempt could not answer. The attempt's internal error text is never here. */
model AutoApplyUnmappedField {
  id: string;
  label: string;
  required: boolean;
  reason: string;
}

/** The live auto-apply attempt of one job, as the application drawer reads it. */
model AutoApplyReviewInfo {
  status: AutoApplyReviewStatus;

  /** Addresses the attempt for the review call. */
  queue_id: int64;

  /** Only when `status` is `pending_review`. */
  resolved_preview?: AutoApplyResolvedPreview;

  /** Only when `status` is `blocked`. */
  unmapped?: AutoApplyUnmappedField[];

  /** Only on a `failed` attempt the employer may already hold; such an attempt is never retried. */
  may_have_reached_employer?: boolean;
}

model TrackedJob {
  added_to_board_at: utcDateTime | null;

  /** Addresses the row: the posting's slug, or `a<id>` for an application whose posting is gone. */
  id: string;

  company_slug: string;
  role_title: string;

  /** The job in the shared wire shape; null once the catalogue has removed the posting. */
  job: unknown;

  viewed_at: utcDateTime | null;
  saved_at: utcDateTime | null;
  applied_at: utcDateTime | null;
  stage: string | null;
  notes: string | null;
  email_count: int64;

  /** The silence fields are null together on any row that is not awaiting a reply. */
  last_activity_at: utcDateTime | null;

  days_silent: int64 | null;
  silence_state: SilenceState | null;
  followed_up_at: utcDateTime | null;
  cv_opened_at: utcDateTime | null;

  /** The status of this job's live auto-apply attempt; absent when it has none. */
  auto_apply_status?: AutoApplyReviewStatus;
}

/** Rows per tab. */
model TrackingCounts {
  all: int64;
  viewed: int64;
  saved: int64;
  applied: int64;
  board: int64;
  dismissed: int64;
}

model TrackingMeta {
  ...PageMeta;
  counts: TrackingCounts;
}

model TrackedJobPage {
  data: TrackedJob[];
  meta: TrackingMeta;
}

/** Observable applications (a connected mailbox would have seen a reply) and how many were answered. */
model ReplyRateSide {
  applications: int64;
  answered: int64;
}

/** The caller's own reply rate against everyone else's, counted the same way. */
model ReplyRateBenchmark {
  you: ReplyRateSide;
  global: ReplyRateSide;
}

/** The application count and the count at each stage, zero included. */
model TrackingPipeline {
  applications: int64;
  stages: Record<int64>;

  /** Applications that ever reached interview or later (current stage or stage history). */
  reached_interview: int64;

  /** Applications that ever reached offer or later (current stage or stage history). */
  reached_offer: int64;

  /** The reply-rate benchmark, only when it clears its sample gate. */
  reply_rate?: ReplyRateBenchmark;
}

model NotificationSettings {
  /** Opt-in Monday summary of application activity. Uses digest_time and account timezone. */
  application_digest_enabled: boolean;
  /** Opt-in daily summary of classified hiring mail. Uses digest_time. */
  inbox_digest_enabled: boolean;
  enabled: boolean;
  channels: string[];

  /**
   * The saved preference. A Free account that kept `instant` or `interval` is sent a daily
   * digest regardless (see `faster_alerts_available`); the preference is kept so a renewal restores it.
   */
  digest_frequency: string;

  /**
   * Whether the account may choose saved-search alerts faster than daily: a live Pro or Ultra
   * plan. When false only `daily` takes effect, and asking for another frequency is refused
   * with 402.
   */
  faster_alerts_available: boolean;

  /** `HH:MM`. */
  digest_time: string | null;

  digest_interval_hours: int64;
  quiet_hours_start: string | null;
  quiet_hours_end: string | null;
}

/** Every field is read as its zero value when left out. Times are `HH:MM`. */
model NotificationSettingsUpdate {
  /** Omitted or null preserves the current weekly application digest choice. */
  application_digest_enabled?: boolean | null;
  /** Omitted or null preserves the current inbox digest choice. */
  inbox_digest_enabled?: boolean | null;
  /** Turn notifications on or off. */
  enabled?: boolean | null;
  /**
   * Delivery channels: `email`, `telegram`, `push`. Adding `push` needs a device the
   * mobile app has registered (409 otherwise).
   */
  channels?: string[] | null;
  digest_frequency?: string | null;
  digest_time?: string | null;
  digest_interval_hours?: int64 | null;
  quiet_hours_start?: string | null;
  quiet_hours_end?: string | null;
}

// Jobs are addressed by their public slug; every write is an idempotent upsert.

@summary("Record that you viewed the job.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/view")
@post
op recordView(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

// The body is optional: without a date the application is stamped now. 400 for a date
// out of range.
/**
 * Send `applied_on` to record an application on the day it was actually sent — importing
 * a history, or correcting a date already stored. A date you state overrides one we
 * inferred. The day is stored at noon UTC, because you are stating a day and midnight
 * reads as the previous date west of Greenwich. A date in the future, older than a year,
 * or not a calendar date, is a 400. Without a body the application is stamped now, as
 * before.
 */
@summary("Mark the job as applied to.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: utcDateTime.fromISO("2026-07-27T12:00:00Z"),
        dismissed_at: null,
        stage: null,
        notes: null,
        application_id: "a17",
      },
    },
})
@route("/jobs/{slug}/apply")
@post
op markApplied(/** The job `public_slug`. */ @path slug: string, @body body?: ApplyJob): Data<AppliedInteraction> | Problem;

@summary("Save (bookmark) the job.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/save")
@post
op saveJob(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

/** Unsaving a job that was never saved answers with the empty interaction. */
@summary("Unsave the job (no-op if not saved).")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/save")
@delete
op unsaveJob(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

/**
 * Only keeps the job out of the swipe deck; it stays visible in the public `/jobs` list
 * and search. Idempotent.
 */
@summary("Dismiss (swipe away) the job.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/dismiss")
@post
op dismissJob(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

@summary("Undismiss the job (no-op if not dismissed).")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/dismiss")
@delete
op undismissJob(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

// 400 for an unknown stage or an empty body.
/**
 * A null field is left unchanged. `stage` is a controlled vocabulary: `preparing`,
 * `applied`, `screening`, `responded`, `interview`, `offer`, `accepted`, `rejected`,
 * `withdrawn`, `expired` (an unknown value is a 400). `expired` is the outcome for an
 * application nobody answered; nothing sets it for you.
 */
@summary("Set the application stage and/or notes.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: "interview",
        notes: "call on Friday",
      },
    },
})
@route("/jobs/{slug}/track")
@patch
op trackJob(/** The job `public_slug`. */ @path slug: string, @body body: TrackJob): Data<Interaction> | Problem;

/** Drops the stage and the apply date; the saved mark, views and notes stay. */
@summary("Clear the application stage.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/stage")
@delete
op clearStage(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

/** Takes the job off the board; the view history stays. */
@summary("Remove the interaction record entirely.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 1,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/jobs/{slug}/track")
@delete
op untrack(/** The job `public_slug`. */ @path slug: string): Data<Interaction> | Problem;

// The same three writes addressed by the row id the listing served: a posting's slug, or
// `a<id>` for an application whose posting was deleted.

/**
 * Same as `PATCH /jobs/{slug}/track`, but addressed by the tracking-board row id instead
 * of the job slug — needed when a tracked application’s posting was later pruned and has
 * no slug left to address it by. Same `stage` vocabulary and body shape.
 */
@summary("Set the application stage and/or notes, addressed by row id.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: "interview",
        notes: null,
      },
    },
})
@route("/me/applications/{id}")
@patch
op trackApplication(/** The tracking row id (from `/me/tracking`). (e.g. `42`) */ @path id: string, @body body: TrackJob): Data<Interaction> | Problem;

@summary("Remove the interaction record entirely, addressed by row id.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 1,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/me/applications/{id}")
@delete
op untrackApplication(/** The tracking row id. (e.g. `42`) */ @path id: string): Data<Interaction> | Problem;

@summary("Clear the application stage, addressed by row id.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        job_id: 42,
        viewed_at: null,
        saved_at: null,
        applied_at: null,
        dismissed_at: null,
        stage: null,
        notes: null,
      },
    },
})
@route("/me/applications/{id}/stage")
@delete
op clearApplicationStage(/** The tracking row id. (e.g. `42`) */ @path id: string): Data<Interaction> | Problem;

// The caller's interactions joined with their jobs, most recently touched first.
// `filter` is all, viewed, saved, applied or board (400 otherwise); `compact` drops
// each card's blurb, skills and ghost signal. A page param that does not read falls
// back to its default.
/**
 * Each item carries a card of the job with your interaction timestamps alongside it —
 * what a list row draws: slug, title, company, closed_at, the stated facets, skills,
 * collections, posted_at, and a `blurb` already cut to length. It does NOT carry the
 * description; the full job view is on `GET /me/tracking/:slug`. `meta.counts` gives the
 * per-filter totals for tab badges. Closed jobs stay listed so your history never
 * shrinks.
 */
@summary("Your tracked jobs joined with the job data.")
@useAuth(SessionOrKey)
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #[
        #{
          added_to_board_at: null,
          id: "senior-go-engineer-acme-1a2b",
          company_slug: "acme",
          role_title: "Senior Go Engineer",
          job: #{
            public_slug: "senior-go-engineer-acme-1a2b",
            title: "Senior Go Engineer",
          },
          viewed_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
          saved_at: null,
          applied_at: utcDateTime.fromISO("2026-06-19T11:00:00Z"),
          stage: "interview",
          notes: "call on Friday",
          email_count: 1,
          last_activity_at: null,
          days_silent: null,
          silence_state: null,
          followed_up_at: null,
          cv_opened_at: null,
        },
      ],
      meta: #{
        total: 5,
        limit: 20,
        offset: 0,
        counts: #{
          all: 12,
          viewed: 12,
          saved: 3,
          applied: 5,
          board: 7,
          dismissed: 1,
        },
      },
    },
})
@route("/me/tracking")
@get
op listTrackedJobs(
  /**
   * Subset to return: `all`, `viewed`, `saved`, `applied`, or `board` (default `all`; an
   * unknown value is a 400). (e.g. `applied`)
   */
  @query filter?: string,
  @query compact?: string,
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): TrackedJobPage | Problem;

/** Every posting the caller has interacted with. */
@summary("Slugs of jobs you have viewed.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #["senior-go-engineer-acme-1a2b", "..."],
  },
})
@tag("Tracking")
@route("/me/tracking/viewed")
@get
op listViewedSlugs(): Data<string[]> | Problem;

/**
 * Lets the SPA render the save toggle as filled without authenticating the public job
 * reads.
 */
@summary("Slugs of jobs you have saved.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #["senior-go-engineer-acme-1a2b", "..."],
  },
})
@tag("Tracking")
@route("/me/tracking/saved")
@get
op listSavedSlugs(): Data<string[]> | Problem;

@summary("The slugs you dismissed, so a client can hide them.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #["junior-php-developer-acme-9z8y"],
  },
})
@tag("Tracking")
@route("/me/tracking/dismissed")
@get
op listDismissedSlugs(): Data<string[]> | Problem;

// The employers the caller has applied to.
/**
 * Lets the SPA mark "you already applied to this company" on any posting of theirs
 * without authenticating the public job reads: compare a posting's `company_slug` against
 * this set. Counts applications actually made (not ones still being prepared), keyed by
 * the posting's current company slug and resolved through company merges. Newest first,
 * drawn from your 1000 most recent applications.
 */
@summary("Company slugs of the employers you have applied to.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #["acme", "globex"],
  },
})
@tag("Tracking")
@route("/me/tracking/applied-companies")
@get
op listAppliedCompanySlugs(): Data<string[]> | Problem;

/**
 * The total application count and the count at each stage, aggregated server-side over
 * all of your applications. Every stage of the vocabulary is present, zero included, and
 * the counts always sum to `applications`. An application with `applied_at` set but no
 * explicit stage counts as `applied`. `reply_rate` compares your own employer-reply rate
 * against the rate across every candidate — both sides counted only from applications
 * whose owner has a connected mailbox, so a reply would have been seen. It is present
 * only once both your own count and the global count reach a minimum sample size; absent
 * otherwise, never a zero or an estimate.
 */
@summary("Your application-pipeline snapshot (counts per stage).")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      applications: 12,
      stages: #{
        applied: 4,
        screening: 2,
        responded: 1,
        interview: 2,
        offer: 1,
        accepted: 1,
        rejected: 1,
        withdrawn: 0,
      },
      reached_interview: 5,
      reached_offer: 2,
      reply_rate: #{
        you: #{
          applications: 12,
          answered: 4,
        },
        global: #{
          applications: 287,
          answered: 97,
        },
      },
    },
  },
})
@tag("Tracking")
@route("/me/tracking/pipeline")
@get
op trackingPipeline(): Data<TrackingPipeline> | Problem;

/** Session cookie only. */
@summary("Your notification rule (gates saved-job reminders and both lifecycle nudges).")
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        application_digest_enabled: false,
        inbox_digest_enabled: true,
        enabled: true,
        channels: #[
          "email",
        ],
        digest_frequency: "daily",
        faster_alerts_available: false,
        digest_time: null,
        digest_interval_hours: 1,
        quiet_hours_start: null,
        quiet_hours_end: null,
      },
    },
})
@route("/me/notification-settings")
@get
op getNotificationSettings(): Data<NotificationSettings> | Problem;

/** An enabled rule needs a valid channel. Session cookie only. */
@summary("Change your notification rule.")
@tag("Tracking")
@opExample(#{
  returnType: #{
      data: #{
        application_digest_enabled: false,
        inbox_digest_enabled: true,
        enabled: true,
        channels: #[
          "email",
        ],
        digest_frequency: "daily",
        faster_alerts_available: false,
        digest_time: null,
        digest_interval_hours: 1,
        quiet_hours_start: null,
        quiet_hours_end: null,
      },
    },
})
@route("/me/notification-settings")
@put
op updateNotificationSettings(@body body: NotificationSettingsUpdate): Data<NotificationSettings> | Problem;
