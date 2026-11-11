import "@typespec/http";

using Http;

namespace Freehire;

// What a signed-in person tells about a job or a company: a vote, or a report that a job
// went silent after they applied. With a session or an API key. An unknown slug is a 404.

model VoteRequest {
  /** `up` or `down`; anything else is a 400. */
  vote?: string;
}

model VoteResult {
  upvote_count: int64;
  downvote_count: int64;

  /** The caller's own vote: 1 up, -1 down, 0 none. */
  my_vote: int64;
}

model GhostReportRequest {
  /** The date the caller applied, YYYY-MM-DD. */
  applied_on?: string;
}

model GhostReport {
  job_id: int64;

  /** YYYY-MM-DD. */
  applied_on: string;

  created_at?: utcDateTime;
}

@tag("Feedback")
@route("/jobs/{slug}/vote")
@post
op voteJob(@path slug: string, @body body: VoteRequest): Data<VoteResult> | Problem;

@tag("Feedback")
@route("/jobs/{slug}/vote")
@delete
op clearJobVote(@path slug: string): Data<VoteResult> | Problem;

@tag("Feedback")
@route("/companies/{slug}/vote")
@post
op voteCompany(@path slug: string, @body body: VoteRequest): Data<VoteResult> | Problem;

@tag("Feedback")
@route("/companies/{slug}/vote")
@delete
op clearCompanyVote(@path slug: string): Data<VoteResult> | Problem;

/**
 * Reports that the job went silent after the caller applied. 403 until the caller's
 * address is confirmed, 409 for a closed job or a second report.
 */
@tag("Feedback")
@route("/jobs/{slug}/ghost-report")
@post
op createGhostReport(@path slug: string, @body body: GhostReportRequest): {
  @statusCode _: 201;
  @body body: Data<GhostReport>;
} | Problem;

/** Withdraws the caller's report. 404 when there is none. */
@tag("Feedback")
@route("/jobs/{slug}/ghost-report")
@delete
op retractGhostReport(@path slug: string): NoContentResponse | Problem;
