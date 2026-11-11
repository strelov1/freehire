import "@typespec/http";

using Http;

namespace Freehire;

// The caller's stored CV and what is derived from it. The verdict, the ATS report, the
// structured CV, the contacts and the coverage reading are derivation or model output
// shared with the assistant and MCP, so each travels as the Go value marshals. Upload
// (`PUT /me/resume`, `POST /me/resume/extract`) and the public roast take a PDF or text
// and stay hand-written.

/** The skill list a coverage reading scores against the market. */
model ResumeCoverageRequest {
  /** Required and at most 100 once blanks are dropped; anything else is a 400. */
  skills?: string[] | null;
}

/**
 * The market-coverage verdict for the caller's profile, scored against the role the
 * query's facet params select. Cookie only; 404 without a profile; 503 when search is off.
 */
@tag("Résumé")
@route("/me/profile/verdict")
@get
op getResumeVerdict(): Data<unknown> | Problem;

/**
 * The ATS-readiness report of the caller's stored CV, with any cached model review folded
 * in. `has_cv` is false, and `report` null, when no CV is stored.
 */
@tag("Résumé")
@route("/me/profile/ats-report")
@get
op getAtsReport(): Data<unknown> | Problem;

/** Runs the model review (paid) over the stored CV, caches it and returns the report. */
@tag("Résumé")
@route("/me/profile/ats-report")
@post
op postAtsReport(): Data<unknown> | Problem;

/** Whether storage is on, a CV is stored, when, and its structured view. Always 200. */
@tag("Résumé")
@route("/me/resume")
@get
op getResume(): Data<unknown> | Problem;

/** Removes the stored CV. 501 when storage is off. */
@tag("Résumé")
@route("/me/resume")
@delete
op deleteResume(): NoContentResponse | Problem;

/** Replaces the owned overrides (contacts, headline, summary, languages, certifications). */
@tag("Résumé")
@route("/me/resume/contacts")
@put
op putResumeContacts(@body body: unknown): Data<unknown> | Problem;

/** Overwrites every owned override from the current structured CV. 409 without one. */
@tag("Résumé")
@route("/me/resume/contacts/replace-from-cv")
@post
op replaceResumeContactsFromCv(): Data<unknown> | Problem;

/** Re-runs the structured extraction over the stored upload. 409 when none is stored. */
@tag("Résumé")
@route("/me/resume/retry-extract")
@post
op retryResumeExtract(): {
  @statusCode _: 202;
} | Problem;

/**
 * Scores a skill list against the market the query's facet params select. The `skills`
 * facet params are not read here and are reported in `meta.ignored_params`.
 */
@tag("Résumé")
@route("/market/coverage")
@post
op marketCoverage(@body body: ResumeCoverageRequest): DataWithIgnored<unknown> | Problem;
