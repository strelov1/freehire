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

// The market-coverage verdict for the caller's profile, scored against the role the
// query's facet params select. Cookie only; 404 without a profile; 503 when search is
// off.
/**
 * How many of your selected role’s open vacancies your skills reach, and which missing
 * skill unlocks the most. The role is the request’s facet params (defaulting to your
 * specializations). No profile is a `404`; `503` when search is unavailable.
 */
@summary("Market-coverage verdict for your profile skills.")
@opExample(#{
  returnType: #{
    data: #{
      total: 1820,
      covered: 1400,
      coverage_percent: 77,
      gaps: #[
        #{
          name: "kubernetes",
          new_vacancies: 120,
          unlock_percent: 7,
        },
      ],
      skills: #[
        #{
          `...`: "...",
        },
      ],
      coherence_percent: 64,
      bundles: #[
        #{
          `...`: "...",
        },
      ],
    },
  },
})
@tag("Résumé")
@route("/me/profile/verdict")
@get
op getResumeVerdict(): Data<unknown> | Problem;

// The ATS-readiness report of the caller's stored CV, with any cached model review
// folded in. `has_cv` is false, and `report` null, when no CV is stored.
/**
 * Scores your stored CV’s structure and its keyword match against the selected role.
 * `has_cv` is false when no CV is stored; no profile is a `404`; `503` when search is
 * unavailable.
 */
@summary("CV ATS-readiness report (deterministic + any cached LLM review).")
@opExample(#{
  returnType: #{
    data: #{
      has_cv: true,
      report: #{
        overall: 78,
        potential: 90,
        categories: #[
          #{
            id: "structure",
            label: "Structure",
            score: 18,
            max: 20,
            items: #[
              #{
                `...`: "...",
              },
            ],
          },
        ],
        strong_keywords: #["go", "postgresql"],
        recommended_keywords: #["kubernetes"],
        reviewed: false,
      },
    },
  },
})
@tag("Résumé")
@route("/me/profile/ats-report")
@get
op getAtsReport(): Data<unknown> | Problem;

// Runs the model review (paid) over the stored CV, caches it and returns the report.
/**
 * Runs the LLM review over your stored CV and folds it into the report
 * (`reviewed: true`). Best-effort: an unconfigured or failing LLM returns the
 * deterministic report (200).
 */
@summary("Run the optional LLM qualitative ATS review and cache it.")
@opExample(#{
  returnType: #{
    data: #{
      has_cv: true,
      report: #{
        overall: 78,
        reviewed: true,
        `...`: "...",
      },
    },
  },
})
@tag("Résumé")
@route("/me/profile/ats-report")
@post
op postAtsReport(): Data<unknown> | Problem;

// Whether storage is on, a CV is stored, when, and its structured view. Always 200.
/**
 * Always `200`: unconfigured storage or no résumé is a normal state. `structured` carries
 * the read-only structured résumé, or null when none is current.
 */
@summary("Your résumé status (enabled / present / uploaded_at).")
@opExample(#{
  returnType: #{
    data: #{
      enabled: true,
      present: true,
      uploaded_at: utcDateTime.fromISO("2026-06-19T10:00:00Z"),
      structured: #{
        `...`: "...",
      },
    },
  },
})
@tag("Résumé")
@route("/me/resume")
@get
op getResume(): Data<unknown> | Problem;

// Removes the stored CV. 501 when storage is off.
/** Returns `204 No Content`. `501` when object storage is unconfigured. */
@summary("Delete your stored résumé.")
@tag("Résumé")
@route("/me/resume")
@delete
op deleteResume(): NoContentResponse | Problem;

// Replaces the owned overrides (contacts, headline, summary, languages,
// certifications).
/**
 * Each body field is owned per field: a value you set here takes over that field from
 * whatever the structured résumé extract would otherwise contribute. Omit a field to
 * leave it as-is; to clear rather than override, use
 * `POST /me/resume/contacts/replace-from-cv`.
 */
@summary("Override one or more contact-block fields on your profile.")
@opExample(#{
  parameters: #{
    body: #{
      email: "me@work.com",
      phone: "+1 555 0100",
    },
  },
  returnType: #{
    data: #{
      full_name: "Jane Doe",
      email: "me@work.com",
      phone: "+1 555 0100",
      location: "Berlin, Germany",
    },
  },
})
@tag("Résumé")
@route("/me/resume/contacts")
@put
op putResumeContacts(@body body: unknown): Data<unknown> | Problem;

// Overwrites every owned override from the current structured CV. 409 without one.
/**
 * A full reset, not just identity fields — every owned field is replaced from the current
 * structured extract. `409` when you have no structured résumé to copy from.
 */
@summary("Reset every contact override from your current structured résumé.")
@opExample(#{
  returnType: #{
    data: #{
      full_name: "Jane Doe",
      email: "jane@example.com",
      location: "Berlin, Germany",
    },
  },
})
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

// Scores a skill list against the market the query's facet params select. The `skills`
// facet params are not read here and are reported in `meta.ignored_params`.
/**
 * Stateless sibling of the CV verdict: skills come from the request body, the market from
 * the facet query params (same vocabulary as search; the `skills` facet is ignored as a
 * filter). Reports how many of the role’s vacancies your skills cover and which missing
 * skill unlocks the most. `400` on empty skills, `503` when search is unavailable.
 */
@summary("Score a supplied skill list against the filtered job market.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      skills: #["go", "postgresql"],
    },
  },
  returnType: #{
    data: #{
      total: 1820,
      covered: 1400,
      coverage_percent: 77,
      gaps: #[
        #{
          name: "kubernetes",
          new_vacancies: 120,
          unlock_percent: 7,
        },
      ],
      skills: #[
        #{
          name: "go",
          market_frequency: 61,
          must_have: true,
          status: "strong",
          advice: "",
        },
      ],
      must_have_total: 8,
      must_have_covered: 6,
      stack_match_percent: 75,
      bundles: #[
        #{
          `...`: "...",
        },
      ],
    },
  },
})
@tag("Résumé")
@route("/market/coverage")
@post
op marketCoverage(@body body: ResumeCoverageRequest): DataWithIgnored<unknown> | Problem;
