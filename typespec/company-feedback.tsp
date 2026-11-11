import "@typespec/http";

using Http;

namespace Freehire;

// A signed-in person's rating of a company, shown under their site-wide pseudonymous
// persona, and the reader-report / moderator-hide lever. Reads are public; writes are
// session-cookie only; the reported queue and hide are moderators only.

/** A company's recomputed feedback counters. */
model CompanyFeedbackSummary {
  feedback_count: int64;

  /** Null until a review counts. */
  feedback_rating_avg: float32 | null;
}

/** One review. The persona handle is the only author identity ever sent. */
model CompanyFeedback {
  id: int64;
  author: string;
  rating: int64;
  feedback_type: string;
  body: string;
  created_at: utcDateTime;
  updated_at: utcDateTime;

  /** The company's fresh counters, set only on the answer to a write. */
  company?: CompanyFeedbackSummary;
}

/** A review with what a moderator triages by. */
model ReportedCompanyFeedback {
  ...CompanyFeedback;
  company_slug: string;
  report_count: int64;
  report_reasons: string[];
}

model CompanyFeedbackPage {
  data: CompanyFeedback[];
  meta: PageMeta;
}

model CompanyFeedbackRequest {
  /** 1 to 5. */
  rating?: int64;

  feedback_type?: string;
  body?: string;
}

model CompanyFeedbackReportRequest {
  reason?: string;
}

/** Newest first. A page param that does not read falls back to its default. */
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@get
op listCompanyFeedback(
  @path slug: string,
  @query limit?: string,
  @query offset?: string,
): CompanyFeedbackPage | Problem;

/** All of the caller's own reviews of the company, one per category; empty when none. */
@tag("Company feedback")
@route("/companies/{slug}/feedback/mine")
@get
op myCompanyFeedback(@path slug: string): Data<CompanyFeedback[]> | Problem;

/** Creates or overwrites the caller's review in one category. */
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@post
op upsertCompanyFeedback(@path slug: string, @body body: CompanyFeedbackRequest): {
  @statusCode _: 201;
  @body body: Data<CompanyFeedback>;
} | Problem;

/** Removes the caller's review in one category; a no-op when there is none. */
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@delete
op deleteCompanyFeedback(@path slug: string, @query feedback_type?: string): Data<CompanyFeedbackSummary> | Problem;

/** The id is declared a string and parsed in Go, which keeps the 400 text. A second report by the same person is a silent no-op. */
@tag("Company feedback")
@route("/company-feedback/{id}/report")
@post
op reportCompanyFeedback(@path id: string, @body body: CompanyFeedbackReportRequest): NoContentResponse | Problem;

/** Moderators only: every review with a report, most reported first. */
@tag("Company feedback")
@route("/company-feedback/reported")
@get
op listReportedCompanyFeedback(): Data<ReportedCompanyFeedback[]> | Problem;

/** Moderators only: hides a review. Idempotent. */
@tag("Company feedback")
@route("/company-feedback/{id}/hide")
@post
op hideCompanyFeedback(@path id: string): NoContentResponse | Problem;
