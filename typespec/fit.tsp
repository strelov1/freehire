import "@typespec/http";

using Http;

namespace Freehire;

// The per-job match surfaces: the deterministic skill match, the on-demand LLM fit
// analysis (cached read and run) and the caller's list of analysed jobs. The run is a paid
// AI route: the paid gate in front of it answers its own 402 before the contract is reached.
// Both `…/stream` routes are Server-Sent Events and stay hand-written. The old `/fit` paths
// are kept as aliases of `/match-analysis`.

/** A job posting scraped off any page. */
model FitMatchTextRequest {
  title?: string;
  text?: string;
}

/**
 * The fit analysis for one job. `has_cv` is false when the caller has no stored CV;
 * `stale` marks a cached analysis whose CV or job changed since; `analysis` is null when
 * none is cached or the model is unconfigured. The allowances ride on reads only and are
 * absent when they cannot be read.
 */
model FitAnalysis {
  has_cv: boolean;
  stale: boolean;
  analysis: unknown;
  allowance?: unknown;
  tailor_allowance?: unknown;
}

model FitAnalysisResponse {
  data: FitAnalysis;
}

/** The 402 a spent or missing fit allowance answers with. */
model FitRefusal {
  error: string;
  allowance: unknown;

  /** Absent for a fair-use refusal and for a caller already on the top plan. */
  upgrade_url?: string;
}

/** One analysed job in the caller's list. */
model FitAnalysedJob {
  slug: string;
  title: string;
  company: string;
  closed: boolean;
  overall_score: int64;
  verdict: string;
  analysed_at: utcDateTime;
  stale: boolean;
}

model FitAnalysedJobsMeta {
  /** Where the caller stands on the fit allowance today; null when it cannot be read. */
  allowance: unknown;
}

model FitAnalysedJobs {
  data: FitAnalysedJob[];
  meta: FitAnalysedJobsMeta;
}

/**
 * How well the job's skills are covered by the caller's profile, plus the advisory
 * hard-constraint blockers. Deterministic, no model. A caller without a profile is a 404.
 */
@tag("Fit")
@route("/jobs/{slug}/match")
@get
op getJobMatch(@path slug: string): Data<unknown> | Problem;

/** The same coverage for a posting that need not be in the catalogue. */
@tag("Fit")
@route("/me/match-text")
@post
op matchText(@body body: FitMatchTextRequest): Data<unknown> | Problem;

/** The cached fit analysis for a job; never calls the model. */
@tag("Fit")
@route("/jobs/{slug}/match-analysis")
@get
op getMatchAnalysis(@path slug: string): FitAnalysisResponse | Problem;

/** Runs the fit analysis and caches it. An unconfigured or failing model answers a null analysis. */
@tag("Fit")
@route("/jobs/{slug}/match-analysis")
@post
op runMatchAnalysis(@path slug: string):
  | FitAnalysisResponse
  | {
      @statusCode _: 402;
      @body body: FitRefusal;
    }
  | Problem;

/** Alias of `getMatchAnalysis`. */
@tag("Fit")
@route("/jobs/{slug}/fit")
@get
op getJobFit(@path slug: string): FitAnalysisResponse | Problem;

/** Alias of `runMatchAnalysis`. */
@tag("Fit")
@route("/jobs/{slug}/fit")
@post
op runJobFit(@path slug: string):
  | FitAnalysisResponse
  | {
      @statusCode _: 402;
      @body body: FitRefusal;
    }
  | Problem;

/** The jobs the caller has run the fit analysis on, newest first. */
@tag("Fit")
@route("/me/tracking/analyses")
@get
op listMyAnalyses(): FitAnalysedJobs | Problem;
