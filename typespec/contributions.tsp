import "@typespec/http";

using Http;

namespace Freehire;

model ResolveJobRequest {
  /** The job page the caller is on; anything not http(s) is a 422. */
  url?: string;

  /** Where the request came from (`extension`, `cli`, …), for the contribution record. */
  surface?: string;
}

/** Where a job page ended up. */
model ResolvedJobPage {
  /** The catalogue job; null while it is still queued. */
  public_slug: string | null;

  /** `found`, `tracked`, `imported`, `review` or `queued`. */
  status: string;

  company_slug?: string;
}

/** A job page the caller sent in, and what became of it. */
model Contribution {
  id: int64;
  url: string;
  source: string;
  board: string;
  status: string;
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
