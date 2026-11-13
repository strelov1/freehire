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

@summary("Vote on a job.")
@useAuth(SessionOrKey)
@tag("Feedback")
@route("/jobs/{slug}/vote")
@post
op voteJob(/** The job `public_slug`. */ @path slug: string, @body body: VoteRequest): Data<VoteResult> | Problem;

@summary("Clear your vote on a job.")
@useAuth(SessionOrKey)
@tag("Feedback")
@route("/jobs/{slug}/vote")
@delete
op clearJobVote(/** The job `public_slug`. */ @path slug: string): Data<VoteResult> | Problem;

@summary("Vote on a company.")
@useAuth(SessionOrKey)
@tag("Feedback")
@route("/companies/{slug}/vote")
@post
op voteCompany(/** The company slug. */ @path slug: string, @body body: VoteRequest): Data<VoteResult> | Problem;

@summary("Clear your vote on a company.")
@useAuth(SessionOrKey)
@tag("Feedback")
@route("/companies/{slug}/vote")
@delete
op clearCompanyVote(/** The company slug. */ @path slug: string): Data<VoteResult> | Problem;

// Reports that the job went silent after the caller applied. 403 until the caller's
// address is confirmed, 409 for a closed job or a second report.
/**
 * An unproven (unverified) address is a `403`; a closed job or a claim you already have
 * on this job is a `409`.
 */
@summary("File a claim that you applied and got no response.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    slug: "<slug>",
    body: #{
      applied_on: "2026-07-29",
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        job_id: 42,
        applied_on: "2026-07-29",
        created_at: utcDateTime.fromISO("2026-07-29T10:00:00Z"),
      },
    },
  },
})
@tag("Feedback")
@route("/jobs/{slug}/ghost-report")
@post
op createGhostReport(/** The job `public_slug`. */ @path slug: string, @body body: GhostReportRequest): {
  @statusCode _: 201;
  @body body: Data<GhostReport>;
} | Problem;

// Withdraws the caller's report. 404 when there is none.
/** A claim that is absent or already withdrawn is a `404`. */
@summary("Withdraw your claim about this job.")
@useAuth(SessionOrKey)
@tag("Feedback")
@route("/jobs/{slug}/ghost-report")
@delete
op retractGhostReport(/** The job `public_slug`. */ @path slug: string): NoContentResponse | Problem;
