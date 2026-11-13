import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The per-job match surfaces: the deterministic skill match, the on-demand LLM fit
// analysis (cached read and run) and the caller's list of analysed jobs. The run is a paid
// AI route: the paid gate in front of it answers its own 402 before the contract is reached.
// Both `…/stream` routes are Server-Sent Events and stay hand-written. The old `/fit` paths
// are kept as aliases of `/match-analysis`.

/** A job posting scraped off any page. */
model FitMatchTextRequest {
  /** The posting title. */
  title?: string;
  /** The scraped posting text. */
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

// How well the job's skills are covered by the caller's profile, plus the advisory
// hard-constraint blockers. Deterministic, no model. A caller without a profile is a
// 404.
/**
 * How well the job’s skills are covered by your profile skills — exact, adjacent, and
 * missing, plus a coverage percent. A caller without a saved profile is a 404.
 */
@summary("Deterministic skill match of the job against your profile (no LLM).")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      total: 12,
      exact_count: 7,
      adjacent_count: 2,
      coverage_percent: 75,
      matched: #["go", "postgresql"],
      adjacent: #[
        #{
          name: "typescript",
          via: "javascript",
        },
      ],
      missing: #["kubernetes"],
    },
  },
})
@tag("Fit")
@route("/jobs/{slug}/match")
@get
op getJobMatch(/** The job `public_slug`. */ @path slug: string): Data<unknown> | Problem;

// The same coverage for a posting that need not be in the catalogue.
/**
 * The same deterministic skill coverage as `GET /jobs/{slug}/match`, but for a page that
 * need not be in the catalogue — this is what lets the extension show a match on any job
 * page. A caller with no profile is a 404. No LLM, and it draws on no allowance.
 */
@summary("Score any job text against your profile.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      title: "Senior Backend Engineer",
      text: "We are looking for …",
    },
  },
  returnType: #{
    data: #{
      coverage_percent: 68,
      matched: #["go", "postgresql"],
      missing: #["kubernetes"],
    },
  },
})
@tag("Fit")
@route("/me/match-text")
@post
op matchText(@body body: FitMatchTextRequest): Data<unknown> | Problem;

// The cached fit analysis for a job; never calls the model.
/**
 * Returns the cached analysis, flagged `stale` when your CV or the job changed since it
 * was computed, or a null analysis when none is cached. `has_cv` is false when you have
 * no stored CV. `allowance` reports how much of today you have used against what the day
 * allows, and when it resets. `tailor_allowance` reports the same for CV tailoring, which
 * carries its own daily ceiling, so a caller deciding between the two has both figures
 * here.
 */
@summary("The cached AI match analysis for the job (never runs the LLM).")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      has_cv: true,
      stale: false,
      analysis: #{
        overall_score: 82,
        verdict: "Strong Fit",
        dimensions: #{
          `...`: "...",
        },
        requirement_match: #[
          #{
            text: "5+ years Go",
            priority: "required",
            status: "covered",
            evidence: "...",
          },
        ],
        strengths: #["..."],
        gaps: #["..."],
        recommendation: "...",
      },
      allowance: #{
        feature: "match",
        used: 1,
        limit: 3,
        unlimited: false,
        enforced: false,
        resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
      },
      tailor_allowance: #{
        feature: "tailor",
        used: 0,
        limit: 2,
        unlimited: false,
        enforced: false,
        resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
      },
    },
  },
})
@tag("Fit")
@route("/jobs/{slug}/match-analysis")
@get
op getMatchAnalysis(/** The job `public_slug`. */ @path slug: string): FitAnalysisResponse | Problem;

// Runs the fit analysis and caches it. An unconfigured or failing model answers a null
// analysis.
/**
 * Runs the match prompt-chain over your stored CV and the job, caches the result, and
 * returns it fresh (no `allowance` on this response). Analysing a new job draws on your
 * daily analysis allowance; with none left it is a `402`, and recomputing an
 * already-analyzed job is free. `has_cv` is false when no CV is stored; a failing or
 * unconfigured LLM returns a null analysis (200).
 */
@summary("Run the three-stage AI match analysis and cache it.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      has_cv: true,
      stale: false,
      analysis: #{
        overall_score: 82,
        verdict: "Strong Fit",
        `...`: "...",
      },
    },
  },
})
@tag("Fit")
@route("/jobs/{slug}/match-analysis")
@post
op runMatchAnalysis(/** The job `public_slug`. */ @path slug: string):
  | FitAnalysisResponse
  | {
      @statusCode _: 402;
      @body body: FitRefusal;
    }
  | Problem;

/** Alias of `getMatchAnalysis`. */
@useAuth(SessionOrKey)
@extension("x-audience", "hidden")
@tag("Fit")
@route("/jobs/{slug}/fit")
@get
op getJobFit(@path slug: string): FitAnalysisResponse | Problem;

/** Alias of `runMatchAnalysis`. */
@useAuth(SessionOrKey)
@extension("x-audience", "hidden")
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

// The jobs the caller has run the fit analysis on, newest first.
/**
 * Newest first, closed jobs included (with `closed: true`). Each item carries the overall
 * score and verdict; `stale` marks an analysis whose CV, job, or model has changed since.
 * `meta.allowance` reports where you stand on the day’s analyses. Never runs the LLM.
 */
@summary("Jobs you have run the AI match analysis on.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        slug: "senior-go-engineer-acme-1a2b",
        title: "Senior Go Engineer",
        company: "Acme",
        closed: false,
        overall_score: 82,
        verdict: "Strong Fit",
        analysed_at: utcDateTime.fromISO("2026-07-11T10:00:00Z"),
        stale: false,
      },
    ],
    meta: #{
      allowance: #{
        feature: "match",
        used: 1,
        limit: 3,
        unlimited: false,
        enforced: false,
        resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
      },
    },
  },
})
@tag("Fit")
@route("/me/tracking/analyses")
@get
op listMyAnalyses(): FitAnalysedJobs | Problem;
