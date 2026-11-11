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

model ApplyJob {
  /** The day the application was sent, `2026-07-29`. Absent, null or empty means now. */
  applied_on?: string | null;
}

/** At least one of the two is required. A field left out or null keeps the stored value. */
model TrackJob {
  stage?: string | null;
  notes?: string | null;
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
  silence_state: string | null;
  followed_up_at: utcDateTime | null;
  cv_opened_at: utcDateTime | null;

  /** The status of this job's live auto-apply attempt; absent when it has none. */
  auto_apply_status?: string;
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

/** The application count and the count at each stage, zero included. */
model TrackingPipeline {
  applications: int64;
  stages: Record<int64>;

  /** The reply-rate benchmark, only when it clears its sample gate. */
  reply_rate?: unknown;
}

model NotificationSettings {
  enabled: boolean;
  channels: string[];
  digest_frequency: string;

  /** `HH:MM`. */
  digest_time: string | null;

  digest_interval_hours: int64;
  quiet_hours_start: string | null;
  quiet_hours_end: string | null;
}

/** Every field is read as its zero value when left out. Times are `HH:MM`. */
model NotificationSettingsUpdate {
  enabled?: boolean | null;
  channels?: string[] | null;
  digest_frequency?: string | null;
  digest_time?: string | null;
  digest_interval_hours?: int64 | null;
  quiet_hours_start?: string | null;
  quiet_hours_end?: string | null;
}

// Jobs are addressed by their public slug; every write is an idempotent upsert.

@tag("Tracking")
@route("/jobs/{slug}/view")
@post
op recordView(@path slug: string): Data<Interaction> | Problem;

/** The body is optional: without a date the application is stamped now. 400 for a date out of range. */
@tag("Tracking")
@route("/jobs/{slug}/apply")
@post
op markApplied(@path slug: string, @body body?: ApplyJob): Data<Interaction> | Problem;

@tag("Tracking")
@route("/jobs/{slug}/save")
@post
op saveJob(@path slug: string): Data<Interaction> | Problem;

/** Unsaving a job that was never saved answers with the empty interaction. */
@tag("Tracking")
@route("/jobs/{slug}/save")
@delete
op unsaveJob(@path slug: string): Data<Interaction> | Problem;

@tag("Tracking")
@route("/jobs/{slug}/dismiss")
@post
op dismissJob(@path slug: string): Data<Interaction> | Problem;

@tag("Tracking")
@route("/jobs/{slug}/dismiss")
@delete
op undismissJob(@path slug: string): Data<Interaction> | Problem;

/** 400 for an unknown stage or an empty body. */
@tag("Tracking")
@route("/jobs/{slug}/track")
@patch
op trackJob(@path slug: string, @body body: TrackJob): Data<Interaction> | Problem;

/** Drops the stage and the apply date; the saved mark, views and notes stay. */
@tag("Tracking")
@route("/jobs/{slug}/stage")
@delete
op clearStage(@path slug: string): Data<Interaction> | Problem;

/** Takes the job off the board; the view history stays. */
@tag("Tracking")
@route("/jobs/{slug}/track")
@delete
op untrack(@path slug: string): Data<Interaction> | Problem;

// The same three writes addressed by the row id the listing served: a posting's slug, or
// `a<id>` for an application whose posting was deleted.

@tag("Tracking")
@route("/me/applications/{id}")
@patch
op trackApplication(@path id: string, @body body: TrackJob): Data<Interaction> | Problem;

@tag("Tracking")
@route("/me/applications/{id}")
@delete
op untrackApplication(@path id: string): Data<Interaction> | Problem;

@tag("Tracking")
@route("/me/applications/{id}/stage")
@delete
op clearApplicationStage(@path id: string): Data<Interaction> | Problem;

/**
 * The caller's interactions joined with their jobs, most recently touched first. `filter` is
 * all, viewed, saved, applied or board (400 otherwise); `compact` drops each card's blurb,
 * skills and ghost signal. A page param that does not read falls back to its default.
 */
@tag("Tracking")
@route("/me/tracking")
@get
op listTrackedJobs(
  @query filter?: string,
  @query compact?: string,
  @query limit?: string,
  @query offset?: string,
): TrackedJobPage | Problem;

/** Every posting the caller has interacted with. */
@tag("Tracking")
@route("/me/tracking/viewed")
@get
op listViewedSlugs(): Data<string[]> | Problem;

@tag("Tracking")
@route("/me/tracking/saved")
@get
op listSavedSlugs(): Data<string[]> | Problem;

@tag("Tracking")
@route("/me/tracking/dismissed")
@get
op listDismissedSlugs(): Data<string[]> | Problem;

/** The employers the caller has applied to. */
@tag("Tracking")
@route("/me/tracking/applied-companies")
@get
op listAppliedCompanySlugs(): Data<string[]> | Problem;

@tag("Tracking")
@route("/me/tracking/pipeline")
@get
op trackingPipeline(): Data<TrackingPipeline> | Problem;

/** Session cookie only. */
@tag("Tracking")
@route("/me/notification-settings")
@get
op getNotificationSettings(): Data<NotificationSettings> | Problem;

/** An enabled rule needs a valid channel. Session cookie only. */
@tag("Tracking")
@route("/me/notification-settings")
@put
op updateNotificationSettings(@body body: NotificationSettingsUpdate): Data<NotificationSettings> | Problem;
