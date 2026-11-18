import "@typespec/http";

using Http;

namespace Freehire;

// The CV builder and the AI tailoring cycle. The CV document and every part of it
// (templates, appearance, revisions, the fit context, ATS and match results) belong to
// Go types the assistant's tools and MCP share, so they travel as raw JSON (`unknown`);
// only the envelopes, ids and small fixed bodies are modelled here. A CV id travels as a
// string: a malformed one is a 404 "cv not found", never a 400. The PDF and the streamed
// cover letter are not JSON and stay hand-written.

/** A CV without its document. */
model CvMeta {
  id: string;
  title: string;
  template_id: string;
  created_at: utcDateTime;
  updated_at: utcDateTime;
}

/** One CV: its document and the state around it. */
model CvRecord {
  ...CvMeta;

  /** The assistant conversation bound to a tailored CV; empty when none. */
  agent_session_id: string;

  /** The structured CV document. */
  document: unknown;

  /** The last unattended run's account of itself; absent when no run has happened. */
  autopilot_report?: unknown;

  /** The candidate's consent for this CV's links to be traced. */
  tracer_links_enabled: boolean;
}

/** A tailored CV in the re-open list. */
model CvTailored {
  ...CvMeta;
  job_slug: string;
  job_title: string;
  job_company: string;
  agent_session_id: string;
}

/** A base CV in the bases list and the tailoring picker. */
model CvBase {
  ...CvMeta;

  /** The base used wherever a single one is needed without asking. */
  is_default: boolean;

  /** The candidate changed it by hand; an upload refresh leaves it alone. */
  hand_edited: boolean;
}

/** Template, typography and margins a new CV starts with; a save replaces the whole set. */
model CvAppearanceDefaults {
  template_id?: string;
  style?: unknown;
  margins?: unknown;
}

model CvAppearanceDefaultsResult {
  template_id: string;
  style: unknown;
  margins: unknown;
}

model CvCreate {
  /** Label; up to 200 characters. */
  title?: string;

  /** Empty falls back to the caller's saved default, then to the system one. */
  template_id?: string;

  /** Pre-fills the CV from the stored résumé structure. */
  seed?: boolean;
}

model CvUpdate {
  /** New label. */
  title?: string;
  /** New template. */
  template_id?: string;
  /** The whole CV document. */
  document?: unknown;
}

model CvTemplateChoice {
  /** A template id from `/cv-templates`. */
  template_id?: string;
}

model CvTracerLinksSetting {
  /** Turn tracing on or off. */
  enabled?: boolean;
}

model CvTracerLinksState {
  tracer_links_enabled: boolean;
}

/** One traced link as the owner reads it. */
model CvTracerLink {
  source_path: string;
  destination_url: string;
  traced_url: string;
  clicks: int64;
  bot_clicks: int64;

  /** Absent when the deployment has no visitor salt. */
  distinct_visitors?: int64;

  last_click_at?: utcDateTime;
}

model CvSessionBinding {
  /** The agent session to bind. */
  session_id?: string;
}

model CvBaseSession {
  base_cv_id: string;
  session_id: string;
}

model CvTailorRequest {
  /** The vacancy to tailor toward. */
  job_slug?: string;

  /** The base to copy; absent means the default base. */
  base_cv_id?: string;
}

model CvTailorResult {
  tailor_cv_id: string;
  base_cv_id: string;

  /** The cached fit analysis, or null. */
  analysis: unknown;

  session_id: string;

  /** True exactly when this call created the tailored CV. */
  cold_start_running: boolean;
}

model CvTailorSession {
  tailor_cv_id: string;
  base_cv_id: string;
  session_id: string;
}

/** A plan refusal: the sentence, the allowance it does not include and the upgrade link. */
model CvPlanRefusal {
  error: string;
  allowance: unknown;
  upgrade_url?: string;
}

/** A stored cover letter; `present` is false, with no letter, when none was drafted. */
model CvCoverLetter {
 evidence_blocked?: boolean;
 evidence_changed?: boolean;
  present: boolean;
  stale?: boolean;
  letter?: unknown;
  cited?: unknown;
  `model`?: string;
}

model CvRevisionUndo {
  cv: CvMeta;
  revision: unknown;
}

model CvAtsDelta {
  available: boolean;
  reason?: string;
  base_cv_id?: string;
  delta?: unknown;
}

model CvJobMatch {
  available: boolean;
  reason?: string;
  score?: unknown;
}

/** The registered CV templates and their display metadata. */
@summary("The template gallery.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: "classic",
        name: "Classic",
      },
      #{
        id: "compact",
        name: "Compact",
      },
    ],
  },
})
@tag("CV")
@route("/cv-templates")
@get
op listCVTemplates(): Data<unknown> | Problem;

// The registered typefaces a CV may choose.
/**
 * Read this rather than hard-coding a list: an unregistered `document.style.font_family`
 * is dropped on save and the CV renders in the template's own face. `note` names the
 * familiar face the entry matches; `css` is a font stack for rendering a preview in a
 * browser.
 */
@summary("The typefaces a CV may use.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: "tinos",
        label: "Tinos",
        note: "Times New Roman metrics",
        css: "Tinos, \"Times New Roman\", Times, serif",
      },
    ],
  },
})
@tag("CV")
@route("/cv-fonts")
@get
op listCVFonts(): Data<unknown> | Problem;

@tag("CV")
@route("/me/cv-appearance-defaults")
@get
op getCVAppearanceDefaults(): Data<CvAppearanceDefaultsResult> | Problem;

@tag("CV")
@route("/me/cv-appearance-defaults")
@put
op setCVAppearanceDefaults(@body body: CvAppearanceDefaults): Data<CvAppearanceDefaultsResult> | Problem;

/** The caller's TAILORED CVs, newest edit first. With a session or an API key. */
@summary("List your CVs, without their documents.")
@useAuth(SessionOrKey)
@tag("CV")
@opExample(#{
  returnType: #{
      data: #[
        #{
          id: "b71e0c3a-5f4d-4c1e-9a2b-3d8f6e7a1c90",
          title: "Backend engineer",
          template_id: "classic",
          created_at: utcDateTime.fromISO("2026-07-01T10:00:00Z"),
          updated_at: utcDateTime.fromISO("2026-07-20T18:30:00Z"),
          job_slug: "senior-go-engineer-acme-1a2b",
          job_title: "Senior Go Engineer",
          job_company: "Acme",
          agent_session_id: "s_9f2a41c7",
        },
      ],
    },
})
@route("/me/cvs")
@get
op listCVs(): Data<CvTailored[]> | Problem;

// The base CVs, oldest first, with the default flagged. With a session or an API key.
/**
 * The non-tailored CVs you can edit, duplicate and tailor from, oldest created first.
 * `is_default` marks the one used when a tailor request names no base; `hand_edited`
 * marks a base you changed in the editor, which a new CV upload will not overwrite. GET
 * /me/cvs lists tailored copies and is unchanged.
 */
@summary("List your base CVs, without their documents.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        id: "0f2c…",
        title: "Backend engineer",
        template_id: "classic-ats",
        is_default: true,
        hand_edited: false,
        created_at: utcDateTime.fromISO("2026-07-01T10:00:00Z"),
        updated_at: utcDateTime.fromISO("2026-07-20T18:30:00Z"),
      },
    ],
  },
})
@tag("CV")
@route("/me/cvs/bases")
@get
op listBaseCVs(): Data<CvBase[]> | Problem;

@summary("Create a CV, optionally seeded from your stored history.")
@opExample(#{
  parameters: #{
    body: #{
      title: "Backend engineer",
      seed: true,
    },
  },
})
@tag("CV")
@route("/me/cvs")
@post
op createCV(@body body: CvCreate): {
  @statusCode _: 201;
  @body body: Data<CvRecord>;
} | Problem;

/** The document without its contact block to an API key. */
@summary("One CV with its full document.")
@useAuth(SessionOrKey)
@tag("CV")
@route("/me/cvs/{id}")
@get
op getCV(/** The CV id. */ @path id: string): Data<CvRecord> | Problem;

@summary("Replace a CV — title, template and document.")
@tag("CV")
@opExample(#{
  returnType: #{
      data: #{
        id: "b71e0c3a-5f4d-4c1e-9a2b-3d8f6e7a1c90",
        title: "Senior Go Engineer",
        template_id: "classic",
        created_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        updated_at: utcDateTime.fromISO("2026-07-28T20:10:00Z"),
      },
    },
})
@route("/me/cvs/{id}")
@put
op updateCV(/** The CV id. */ @path id: string, @body body: CvUpdate): Data<CvMeta> | Problem;

@summary("Delete a CV.")
@tag("CV")
@route("/me/cvs/{id}")
@delete
op deleteCV(/** The CV id. */ @path id: string): NoContentResponse | Problem;

@summary("Switch the template only.")
@tag("CV")
@route("/me/cvs/{id}/template")
@put
op setCVTemplate(/** The CV id. */ @path id: string, @body body: CvTemplateChoice): NoContentResponse | Problem;

// 409 when enabling on a deployment with no visitor salt.
/**
 * Off is the default for every CV. Turning it on is refused with a `409` on a deployment
 * with no visitor salt configured — there would be no honest way to count distinct
 * visitors. Turning it off is never refused.
 */
@summary("Turn link tracing on or off for this CV's PDF.")
@opExample(#{
  parameters: #{
    id: "0f2c…",
    body: #{
      enabled: true,
    },
  },
  returnType: #{
    data: #{
      tracer_links_enabled: true,
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/tracer-links")
@put
op setCVTracerLinks(/** The CV id. */ @path id: string, @body body: CvTracerLinksSetting): Data<CvTracerLinksState> | Problem;

/**
 * One entry per link the PDF carries (header links, project links). Clicks you made on
 * your own CV are excluded from the counts; `bot_clicks` is tallied separately rather
 * than folded in. `distinct_visitors` is omitted — not zero — on a deployment with no
 * visitor salt, since there is no honest count to report.
 */
@summary("What is known about this CV's traced links.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        source_path: "header.links[0]",
        destination_url: "https://github.com/you",
        traced_url: "https://freehire.me/cv/acme-x7abc",
        clicks: 12,
        bot_clicks: 3,
        distinct_visitors: 9,
        last_click_at: utcDateTime.fromISO("2026-07-20T10:00:00Z"),
      },
    ],
  },
})
@tag("CV")
@route("/me/cvs/{id}/tracer-links")
@get
op listCVTracerLinks(/** The CV id. */ @path id: string): Data<CvTracerLink[]> | Problem;

// Copies a base CV as "<title> (copy)"; 409 for a tailored one.
/**
 * Creates a new base CV with the same document, template and appearance, titled "<title>
 * (copy)". Only a base can be duplicated: 409 for a tailored copy, 404 when the id is not
 * yours.
 */
@summary("Copy a base CV.")
@tag("CV")
@route("/me/cvs/{id}/duplicate")
@post
op duplicateCV(/** The base CV id. */ @path id: string): {
  @statusCode _: 201;
  @body body: Data<CvRecord>;
} | Problem;

// The assistant conversation for a base CV; not behind the paid gate.
/**
 * Returns the assistant conversation bound to one of your base CVs, creating it on first
 * use. It is the tailoring workspace's agent without a vacancy: it edits this general CV
 * through the same evidence-gated tools. Opening it is free and needs no paid plan; each
 * message still passes the assistant's paid gate. 409 for a tailored copy (use the tailor
 * session), 404 when the id is not yours.
 */
@summary("Open the agent conversation for a base CV.")
@opExample(#{
  returnType: #{
    data: #{
      base_cv_id: "0f2c…",
      session_id: "s_9f…",
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/base-session")
@post
op startBaseSession(/** The base CV id. */ @path id: string): Data<CvBaseSession> | Problem;

// Starts the tailoring session for a vacancy (the paid gate runs before this). 201 with
// the ids and the cached analysis; 402 when the allowance is spent and the vacancy is
// new. With a session or an API key.
/**
 * Copies your base CV into a vacancy-bound tailored one and mints the short-lived
 * credential its agent session runs on. Requires a cached match analysis for the job (409
 * otherwise) and a base CV — one is seeded from your stored history when you have none,
 * and it is a 409 when there is nothing to seed from. Calls no LLM itself.
 */
@summary("Start tailoring a CV toward one vacancy.")
@useAuth(SessionOrKey)
@tag("CV")
@opExample(#{
  returnType: #{ _: 201, body: #{
      data: #{
        tailor_cv_id: "7d1a3b5c-2e4f-4a6b-8c9d-0e1f2a3b4c5d",
        base_cv_id: "0f2c9d1e-7a3b-4e5f-8c6d-2b1a0e9f8d7c",
        analysis: #{
          overall_score: 72,
          verdict: "worth_applying",
        },
        session_id: "s_9f2a41c7",
        cold_start_running: true,
      },
    } },
})
@route("/me/cvs/tailor")
@post
op tailorCV(@body body: CvTailorRequest):
  | {
      @statusCode _: 201;
      @body body: Data<CvTailorResult>;
    }
  | {
      @statusCode _: 402;
      @body body: CvPlanRefusal;
    }
  | Problem;

/**
 * For a tailored CV whose session was lost or never bound. 409 when the CV is not a
 * tailored copy.
 */
@summary("Mint a fresh agent session for an existing tailored CV.")
@opExample(#{
  returnType: #{
    _: 201,
    body: #{
      data: #{
        tailor_cv_id: "7d1a…",
        base_cv_id: "0f2c…",
        session_id: "s_9f…",
      },
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}/tailor-session")
@post
op startTailorSession(/** The tailored CV id. */ @path id: string): {
  @statusCode _: 201;
  @body body: Data<CvTailorSession>;
} | Problem;

// Rebuilds the default base CV from the current seed.
/**
 * Replaces the base (non-tailored) document from the current seed (experience bank first,
 * your résumé's own extract for what the bank doesn't track). Preserves template and
 * typography. Does not rewrite tailored copies for specific jobs. 409 when there is no
 * usable seed.
 */
@summary("Rebuild your base CV from your current seed.")
@tag("CV")
@route("/me/cvs/base/reseed")
@post
op reseedBaseCV(): Data<CvRecord> | Problem;

// Rebuilds one CV from the current seed; destructive.
/**
 * Replaces this CV's content from the current seed (experience bank first, your résumé's
 * own extract for what the bank doesn't track). Works on a tailored copy or on a base,
 * and touches only that CV: refreshing a tailored copy no longer rewrites any base, and
 * refreshing a base — the editor's "Update from my CV", which replaces hand edits —
 * leaves its copies alone. Keeps the same id and agent session; preserves template and
 * typography. 409 when there is no usable seed. Upload alone does not do this — this is
 * the explicit apply.
 */
@summary("Rebuild one CV from your current seed.")
@tag("CV")
@route("/me/cvs/{id}/reseed")
@post
op reseedCV(/** The CV id (tailored or base). */ @path id: string): Data<CvRecord> | Problem;

// Applies a batch of path operations (`{"ops": [...], "note": ""}`), decoded strictly
// in Go: an unknown field or bad addressing is a 422. With a session or an API key.
/**
 * An edit is a `kind` (`set`, `insert`, `remove`, `move`) and a `path` into the CV —
 * `summary`, `experience[2].bullets[1]`, `skills[0].items[3]`, `style.font_size`. Indices
 * are 0-based. The whole batch applies or none of it does: an unknown path or an index
 * past the end refuses the request without touching the CV. An API key edits as the
 * tailoring agent, so the candidate's own fields are refused and a claim about them needs
 * `evidence_id` from the experience bank. Every request lands in the CV's history and can
 * be undone on its own.
 */
@summary("Apply a batch of edits, addressed by path.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    id: "0f2c…",
    body: #{
      ops: #[
        #{
          kind: "set",
          path: "experience[0].bullets[1]",
          value: "Cut p99 checkout latency 40% by …",
          evidence_id: "a71f…",
        },
      ],
    },
  },
})
@tag("CV")
@route("/me/cvs/{id}")
@patch
op patchCV(/** The CV id. */ @path id: string, @body body: unknown): Data<CvMeta> | Problem;

@summary("Bind an agent session to a CV.")
@useAuth(SessionOrKey)
@tag("CV")
@route("/me/cvs/{id}/session")
@put
op setCVSession(/** The CV id. */ @path id: string, @body body: CvSessionBinding): NoContentResponse | Problem;
