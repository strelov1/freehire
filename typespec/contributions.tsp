import "@typespec/http";

using Http;

namespace Freehire;

model ResolveJobRequest {
  /** The job page the caller is on; anything not http(s) is a 422. */
  url?: string;

  /** Where the request came from (`extension`, `cli`, …), for the contribution record. */
  surface?: string;
}

/**
 * What became of the board behind a job page: `found` (the catalogue already carried the
 * posting), `tracked` (imported, and the board is already crawled), `imported` (imported,
 * and the board queued for onboarding), `review` (imported, but no crawlable board could be
 * named) or `queued` (nothing could read the page, so the link went to manual triage).
 */
enum ResolvedJobStatus {
  found,
  tracked,
  imported,
  review,
  queued,
}

/** Where a job page ended up. */
model ResolvedJobPage {
  /** The catalogue job; null while it is still queued. */
  public_slug: string | null;

  status: ResolvedJobStatus;

  company_slug?: string;
}

/**
 * `pending`: recorded, not yet proven by a crawl; `active`: its first crawl succeeded;
 * `rejected` and `retired`: a curator turned it down or removed it; `review`: an
 * unrecognised link awaiting manual triage.
 */
enum ContributionStatus {
  pending,
  active,
  rejected,
  retired,
  review,
}

/** A job page the caller sent in, and what became of it. */
model Contribution {
  id: int64;
  url: string;
  source: string;
  board: string;
  status: ContributionStatus;
  surface: string;
  created_at: utcDateTime | null;
}

/**
 * Finds the catalogue job behind a page, or imports it. 200 when it was already
 * there, 201 when this request created it, 202 when it is queued for later.
 * With a session or an API key.
 */
@tag("Contributions")
@route("/jobs/resolve")
@post
op resolveJob(@body body: ResolveJobRequest):
  | {
      @statusCode _: 200;
      @body body: Data<ResolvedJobPage>;
    }
  | {
      @statusCode _: 201;
      @body body: Data<ResolvedJobPage>;
    }
  | {
      @statusCode _: 202;
      @body body: Data<ResolvedJobPage>;
    }
  | Problem;

@tag("Contributions")
@route("/me/contributions")
@get
op listMyContributions(): Data<Contribution[]> | Problem;
