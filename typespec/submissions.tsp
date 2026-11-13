import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The public link-submission queue: any signed-in user (cookie or API key) submits a
// vacancy for review and reads their own queue; the pending queue, approve and reject
// are moderator-only.

enum SubmissionStatus {
  pending,
  approved,
  rejected,
}

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

  status: SubmissionStatus;

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
  /** Link to the original posting. (e.g. `https://acme.com/careers/123`) */
  url?: string | null;
  /** Origin hint (optional). */
  source?: string | null;
  /** Job title. (e.g. `Senior Go Engineer`) */
  title?: string | null;
  /** Company name. (e.g. `Acme`) */
  company?: string | null;
  /** Free-text location. (e.g. `Remote — EU`) */
  location?: string | null;
  /** Whether the role is remote. (e.g. `true`) */
  remote?: boolean | null;
  /** Job description. */
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
  /** The job posting URL to parse. */
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

// 409 for a duplicate pending URL or a vacancy already in the catalogue, 403 for a
// blocked domain.
/**
 * A URL already awaiting review is a `409`, and so is a URL the catalogue already carries
 * — matched the way `/jobs/find` matches it, so a link differing only by scheme, `www.`,
 * tracking query or trailing slash counts as the same posting. That `409` names the
 * existing job (`/jobs/<public_slug>`) in its error.
 */
@summary("Submit a vacancy for review.")
@useAuth(SessionOrKey)
@tag("Submissions")
@route("/submissions")
@post
op createSubmission(@body body: SubmissionRequest): {
  @statusCode _: 201;
  @body body: Data<Submission>;
} | Problem;

// Writes nothing. An unrecognised URL answers 200 with every field empty.
/**
 * Uses the same ATS-recognition registry as `/jobs/resolve`, but writes nothing — no job,
 * no submission, no allowance spent. An unrecognized URL returns an empty object rather
 * than an error. A URL the catalogue already carries returns only `existing_job_slug`,
 * and `POST /submissions` would refuse it.
 */
@summary("Parse a job URL into a draft submission for review before posting.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      url: "https://boards.greenhouse.io/acme/jobs/123",
    },
  },
  returnType: #{
    data: #{
      title: "Senior Go Engineer",
      company: "Acme",
      location: "Remote — EU",
      description: "...",
      work_mode: "remote",
      employment_type: "full_time",
      seniority: "senior",
      skills: #["go"],
      source: "greenhouse",
    },
  },
})
@tag("Submissions")
@route("/submissions/prefill")
@post
op prefillSubmission(@body body: PrefillRequest): Data<SubmissionPrefill> | Problem;

@summary("Your own submission queue.")
@useAuth(SessionOrKey)
@tag("Submissions")
@route("/me/submissions")
@get
op listMySubmissions(): Data<Submission[]> | Problem;

/** Moderators only. */
@summary("The pending submission queue (moderators).")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Submissions")
@route("/submissions")
@get
op listPendingSubmissions(): Data<Submission[]> | Problem;

/** Moderators only. 404 for an unknown id, 409 for one already decided. */
@summary("Approve a submission, minting a live job.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Submissions")
@route("/submissions/{id}/approve")
@post
op approveSubmission(/** The submission id. (e.g. `9`) */ @path id: string): Data<Submission> | Problem;

/** Moderators only. The body is optional. */
@summary("Reject a submission with a reason.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Submissions")
@route("/submissions/{id}/reject")
@post
op rejectSubmission(/** The submission id. (e.g. `9`) */ @path id: string, @body body?: RejectSubmissionRequest): Data<Submission> | Problem;
