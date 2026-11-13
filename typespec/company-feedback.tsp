import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

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

  /** Review category, e.g. `interview`, `culture`, `compensation`. (e.g. `interview`) */
  feedback_type?: string;
  /** Free-text review. (e.g. `Fast process, clear communication.`) */
  body?: string;
}

model CompanyFeedbackReportRequest {
  /** Report reason code. (e.g. `spam`) */
  reason?: string;
}

/** Newest first. A page param that does not read falls back to its default. */
@summary("List a company's feedback, newest first.")
@useAuth(NoAuth)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: 5,
        author: "quiet-falcon-42",
        rating: 4,
        feedback_type: "interview",
        body: "Fast process, clear communication.",
        created_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        updated_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
      },
    ],
    meta: #{
      total: 12,
      limit: 20,
      offset: 0,
    },
  },
})
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@get
op listCompanyFeedback(
  /** The company slug. (e.g. `acme`) */
  @path slug: string,
  /** Page size, 1–100. (e.g. `20`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): CompanyFeedbackPage | Problem;

/** All of the caller's own reviews of the company, one per category; empty when none. */
@summary("Your own feedback on this company, across every category (empty if none).")
@tag("Company feedback")
@route("/companies/{slug}/feedback/mine")
@get
op myCompanyFeedback(/** The company slug. (e.g. `acme`) */ @path slug: string): Data<CompanyFeedback[]> | Problem;

// Creates or overwrites the caller's review in one category.
/**
 * One entry per (user, category): posting again in the same `feedback_type` overwrites
 * your existing review rather than adding a second one. The response's nested `company`
 * field is the freshly recomputed rating count/average, so you can update your own view
 * of the company without a second fetch. `422` on an empty body.
 */
@summary("Create or overwrite your feedback in one category on a company.")
@opExample(#{
  parameters: #{
    slug: "acme",
    body: #{
      rating: 4,
      feedback_type: "interview",
      body: "Fast process, clear communication.",
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        id: 5,
        author: "quiet-falcon-42",
        rating: 4,
        feedback_type: "interview",
        body: "Fast process, clear communication.",
        created_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        updated_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        company: #{
          feedback_count: 12,
          feedback_rating_avg: 4.1,
        },
      },
    },
  },
})
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@post
op upsertCompanyFeedback(/** The company slug. (e.g. `acme`) */ @path slug: string, @body body: CompanyFeedbackRequest): {
  @statusCode _: 201;
  @body body: Data<CompanyFeedback>;
} | Problem;

// Removes the caller's review in one category; a no-op when there is none.
/**
 * Returns the company's freshly recomputed counters, the same cast/clear shape as the
 * vote endpoints.
 */
@summary("Delete your feedback in one category (no-op if absent).")
@opExample(#{
  returnType: #{
    data: #{
      feedback_count: 11,
      feedback_rating_avg: 4,
    },
  },
})
@tag("Company feedback")
@route("/companies/{slug}/feedback")
@delete
op deleteCompanyFeedback(/** The company slug. (e.g. `acme`) */ @path slug: string, /** Which category to delete. (e.g. `interview`) */ @query feedback_type?: string): Data<CompanyFeedbackSummary> | Problem;

// The id is declared a string and parsed in Go, which keeps the 400 text. A second
// report by the same person is a silent no-op.
/** A second report of the same review by you is a silent no-op. */
@summary("Report a specific review.")
@tag("Company feedback")
@route("/company-feedback/{id}/report")
@post
op reportCompanyFeedback(/** The feedback entry id. (e.g. `5`) */ @path id: string, @body body: CompanyFeedbackReportRequest): NoContentResponse | Problem;

/** Moderators only: every review with a report, most reported first. */
@summary("Every review with at least one report, most-reported first.")
@extension("x-audience", "moderator")
@tag("Company feedback")
@route("/company-feedback/reported")
@get
op listReportedCompanyFeedback(): Data<ReportedCompanyFeedback[]> | Problem;

// Moderators only: hides a review. Idempotent.
/** Idempotent. 404 for an unknown id. */
@summary("Hide a review, dropping it from the company's public list and average.")
@extension("x-audience", "moderator")
@tag("Company feedback")
@route("/company-feedback/{id}/hide")
@post
op hideCompanyFeedback(/** The feedback entry id. (e.g. `5`) */ @path id: string): NoContentResponse | Problem;
