import "@typespec/http";

using Http;

namespace Freehire;

// The public link-submission queue: any signed-in user (cookie or API key) submits a
// vacancy for review and reads their own queue; the pending queue, approve and reject
// are moderator-only.

/** A submission as the API shows it. */
model Submission {
  id: int64;
  url: string;
  source?: string;
  title: string;
  company: string;
  location?: string;
  remote: boolean;
  description?: string;

  /** RFC 3339. */
  posted_at?: string;

  /** pending, approved or rejected. */
  status: string;

  review_reason?: string;
  reviewed_at?: string;
  created_at: string | null;

  /** Moderator queue only. */
  submitter_email?: string;

  /** The minted vacancy's slug; on an approved submission in the caller's own list only. */
  job_slug?: string;

  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: int64;
  salary_max?: int64;
  salary_currency?: string;
  salary_period?: string;
}

/** A vacancy to submit. url, title and company are required; the service refuses a body without them with a 400. */
model SubmissionRequest {
  url?: string | null;
  source?: string | null;
  title?: string | null;
  company?: string | null;
  location?: string | null;
  remote?: boolean | null;
  description?: string | null;

  /** RFC 3339. */
  posted_at?: string | null;

  skills?: string[] | null;
  regions?: string[] | null;
  cities?: string[] | null;
  work_mode?: string | null;
  employment_type?: string | null;
  seniority?: string | null;
  salary_min?: int64 | null;
  salary_max?: int64 | null;
  salary_currency?: string | null;
  salary_period?: string | null;
}

model PrefillRequest {
  url?: string;
}

/** What could be parsed from the URL. Empty when it is not recognised. */
model SubmissionPrefill {
  title?: string;
  company?: string;
  location?: string;
  description?: string;
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  skills?: string[];
  source?: string;

  /** Set, with every other field empty, when the catalogue already carries the vacancy. */
  existing_job_slug?: string;
}

model RejectSubmissionRequest {
  reason?: string;

  /** Also blocks the URL's host and rejects every other pending submission on it. */
  block_domain?: boolean;
}

/** 409 for a duplicate pending URL or a vacancy already in the catalogue, 403 for a blocked domain. */
@tag("Submissions")
@route("/submissions")
@post
op createSubmission(@body body: SubmissionRequest): {
  @statusCode _: 201;
  @body body: Data<Submission>;
} | Problem;

/** Writes nothing. An unrecognised URL answers 200 with every field empty. */
@tag("Submissions")
@route("/submissions/prefill")
@post
op prefillSubmission(@body body: PrefillRequest): Data<SubmissionPrefill> | Problem;

@tag("Submissions")
@route("/me/submissions")
@get
op listMySubmissions(): Data<Submission[]> | Problem;

/** Moderators only. */
@tag("Submissions")
@route("/submissions")
@get
op listPendingSubmissions(): Data<Submission[]> | Problem;

/** Moderators only. 404 for an unknown id, 409 for one already decided. */
@tag("Submissions")
@route("/submissions/{id}/approve")
@post
op approveSubmission(@path id: string): Data<Submission> | Problem;

/** Moderators only. The body is optional. */
@tag("Submissions")
@route("/submissions/{id}/reject")
@post
op rejectSubmission(@path id: string, @body body?: RejectSubmissionRequest): Data<Submission> | Problem;
