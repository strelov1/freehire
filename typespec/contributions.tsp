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

// Finds the catalogue job behind a page, or imports it. 200 when it was already there,
// 201 when this request created it, 202 when it is queued for later. With a session or
// an API key.
/**
 * Five outcomes in one shape, distinguished by status: **200 found** — the catalogue
 * already carries the vacancy. Either the URL itself is stored (nothing is fetched or
 * written), or the page turned out to be a second copy of a posting we already carry
 * under another source, in which case the answer names the one we had; **201 tracked** —
 * we crawl that board already and the posting just had not landed, so it was imported
 * now; **201 imported** — imported, and its board queued for onboarding; **201 review** —
 * imported, but its careers site names no board we can crawl, so the link went to manual
 * triage; **202 queued** — nothing could read the page, so the link went to manual
 * triage. `company_slug` is returned whenever the catalogue already carries the employer,
 * through any source — independent of what became of the board. A URL that is not http(s)
 * is a 422.
 */
@summary("Hand freehire a link: import the posting, and the board behind it.")
@useAuth(SessionOrKey)
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

@summary("The boards you contributed, and their onboarding state.")
@useAuth(SessionOrKey)
@tag("Contributions")
@route("/me/contributions")
@get
op listMyContributions(): Data<Contribution[]> | Problem;
