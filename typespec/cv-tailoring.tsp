import "@typespec/http";

using Http;

namespace Freehire;

// The caller's CVs, continued from cv.tsp: what the tailoring workspace reads beside a
// tailored CV — the tailoring context, the cover letter and its evidence, the edit
// history and its undo, the ATS delta and the job match.

// The cached fit analysis of a tailored CV, projected for the tailoring agent.
/**
 * The split that keeps tailoring honest: `missing_have` are requirements your history
 * already covers but the CV buries — reframe those — and `missing_gap` are the ones it
 * does not, which an agent must ask about rather than invent. Served from cache; calls no
 * LLM. 409 when the CV is not a tailored copy or has no analysis. `job.requirements` is
 * what the posting itself asks for, in its own words — a fuller list than the missing_*
 * split, which is only what the analysis found the CV lacks. Absent when the posting
 * states none.
 */
@summary("The match analysis a tailored CV should reframe toward.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      job: #{
        slug: "senior-backend-engineer-acme-1a2b",
        title: "Senior Backend Engineer",
        company: "Acme",
        requirements: #[
          #{
            text: "5+ years with Go",
            priority: "required",
          },
        ],
      },
      verdict: "worth_applying",
      overall_score: 72,
      recommendation: "Lead with the payments migration …",
      missing_have: #[
        #{
          requirement: "Kafka at scale",
          evidence: "Ran the event bus at …",
        },
      ],
      missing_gap: #[
        #{
          requirement: "Kubernetes operators",
        },
      ],
      strengths: #["Go", "payments"],
      gaps: #["k8s operators"],
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/tailor-context")
@get
op getCVTailorContext(/** The tailored CV id. */ @path id: string): Data<unknown> | Problem;

/** Never calls a model. With a session or an API key. */
@useAuth(SessionOrKey)
@tag("CV")
@route("/me/cvs/{id}/cover-letter")
@get
op getCVCoverLetter(@path id: string): Data<CvCoverLetter> | Problem;

/** Drafts the letter (the paid gate runs before this); 402 when the allowance is spent. */
@tag("CV")
@route("/me/cvs/{id}/cover-letter")
@post
op draftCVCoverLetter(@path id: string, @query band?: string):
  | {
      @statusCode _: 200;
      @body body: Data<CvCoverLetter>;
    }
  | {
      @statusCode _: 402;
      @body body: CvPlanRefusal;
    }
  | Problem;

// What changed the CV, newest first.
/**
 * Each entry names what changed and where (`paths`), not the edit itself — the operations
 * carry your own prior text and stay server-side. `undoable` is false for an entry with
 * nothing to reverse (e.g. the CV’s own creation); `reverted` marks an entry already
 * undone rather than removing it from the feed.
 */
@summary("The edit history of this CV, newest first.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: "b1a2…",
        actor: "agent",
        origin: "tailoring",
        batch_id: "f3c1…",
        title: "Reframed 2 bullets toward Kafka",
        note: "Emphasized event-bus scale to match the requirement",
        paths: #["experience[0].bullets[1]"],
        reverted: false,
        undoable: true,
        created_at: utcDateTime.fromISO("2026-07-28T20:10:00Z"),
      },
    ],
  },
})
@tag("CV")
@route("/me/cvs/{id}/revisions")
@get
op listCVRevisions(/** The CV id. */ @path id: string): Data<unknown> | Problem;

/**
 * The undo is itself recorded as a new revision. A `409` when the entry has nothing to
 * reverse or the part of the document it changed is already gone.
 */
@summary("Reverse one revision, leaving later edits in place.")
@opExample(#{
  returnType: #{
    data: #{
      cv: #{
        id: "0f2c…",
        title: "Backend engineer",
        template_id: "classic",
        created_at: utcDateTime.fromISO("2026-07-01T10:00:00Z"),
        updated_at: utcDateTime.fromISO("2026-07-28T20:15:00Z"),
      },
      revision: #{
        id: "c4d5…",
        actor: "user",
        origin: "editor",
        title: "Undid \"Reframed 2 bullets toward Kafka\"",
        reverts_id: "b1a2…",
        reverted: false,
        undoable: false,
        paths: #["experience[0].bullets[1]"],
        created_at: utcDateTime.fromISO("2026-07-28T20:15:00Z"),
      },
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/revisions/{rid}/undo")
@post
op undoCVRevision(/** The CV id. */ @path id: string, /** The revision id, from the history feed. */ @path rid: string): Data<CvRevisionUndo> | Problem;

/**
 * `bid` is a revision’s `batch_id` from the history feed, not a separate id — undoing one
 * entry of a batch and then the batch itself is well-defined, since an already-reverted
 * entry is simply skipped.
 */
@summary("Reverse every standing edit of one agent turn, newest first.")
@opExample(#{
  returnType: #{
    data: #{
      id: "0f2c…",
      title: "Backend engineer",
      template_id: "classic",
      created_at: utcDateTime.fromISO("2026-07-01T10:00:00Z"),
      updated_at: utcDateTime.fromISO("2026-07-28T20:16:00Z"),
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/revisions/batch/{bid}/undo")
@post
op undoCVRevisionBatch(/** The CV id. */ @path id: string, /** The batch id shared by the revisions to undo. */ @path bid: string): Data<CvMeta> | Problem;

/**
 * The tailored copy scored against the base CV it came from, with template, page margins
 * and typography held identical so the difference reflects content alone — the base is a
 * copy rendered with the tailored copy’s formatting, never the stored base itself.
 * Recomputed per request, never stored. `available: false` (with `reason`, no `delta`)
 * when rendering is unavailable rather than an error. `409` when the CV is not a tailored
 * copy, has no base CV to compare against, or its vacancy no longer exists.
 */
@summary("How tailoring changed this CV's ATS-readiness score.")
@opExample(#{
  returnType: #{
    data: #{
      available: true,
      base_cv_id: "0f2c…",
      delta: #{
        base: 62,
        tailored: 78,
        change: 16,
        categories: #[
          #{
            id: "keyword_coverage",
            label: "Keyword coverage",
            `...`: "...",
          },
        ],
        regressed: false,
      },
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/ats-delta")
@get
op getCVATSDelta(/** The tailored CV id. */ @path id: string): Data<CvAtsDelta> | Problem;

/**
 * Deterministic and free — no model call, unlike the AI match analysis. Scored off the
 * CV’s rendered PDF text layer against the vacancy alone (no base-CV comparison, unlike
 * the ATS delta), so it is cheap enough to refresh after every saved edit.
 * `available: false` when nothing about the vacancy could be matched automatically, or
 * rendering failed. `409` when the CV is not a tailored copy or its vacancy no longer
 * exists.
 */
@summary("How well a tailored CV matches the vacancy it was written for.")
@opExample(#{
  returnType: #{
    data: #{
      available: true,
      score: #{
        overall: 74,
        categories: #[
          #{
            `...`: "...",
          },
        ],
        contributing: #["skills", "requirements"],
        matched_skills: #["go", "postgresql"],
        missing_skills: #["kubernetes"],
        requirements: #[
          #{
            text: "5+ years Go",
            priority: "required",
            cached_status: "covered",
          },
        ],
      },
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/job-match")
@get
op getCVJobMatch(/** The tailored CV id. */ @path id: string): Data<CvJobMatch> | Problem;

/** Review confirmed evidence without a model call or allowance charge. */
@summary("Review the confirmed evidence for a cover letter.")
@opExample(#{
  parameters: #{ id: "b71e0c3a-5f4d-4c1e-9a2b-3d8f6e7a1c90" },
  returnType: #{ data: #{ revision: 0, explicit: false, atom_ids: #[], suggested_ids: #[], invalid_ids: #[], available: #[] } },
})
@useAuth(SessionOrKey)
@tag("CV")
@route("/me/cvs/{id}/cover-letter/evidence")
@get
op getCVCoverLetterEvidence(/** The tailored CV id. */ @path id: string): Data<unknown> | Problem;

/**
 * Save up to 50 confirmed achievement ids with an expected revision.
 * An empty atom_ids array explicitly prevents drafting until facts are selected.
 * Returns 409 when the expected revision is stale, or 422 for invalid evidence.
 */
@summary("Save the confirmed evidence selection for a cover letter.")
@opExample(#{
  parameters: #{ id: "b71e0c3a-5f4d-4c1e-9a2b-3d8f6e7a1c90", body: #{ revision: 0, atom_ids: #[] } },
  returnType: #{ data: #{ revision: 1, atom_ids: #[] } },
})
@tag("CV")
@route("/me/cvs/{id}/cover-letter/evidence")
@put
op saveCVCoverLetterEvidence(
  /** The tailored CV id. */ @path id: string,
  /** revision is the expected saved revision (0 before the first save); atom_ids contains up to 50 unique, owned, confirmed achievement UUIDs. */
  @body body: unknown,
): Data<unknown> | Problem;
