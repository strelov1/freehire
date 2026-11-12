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
  title?: string;

  /** Empty falls back to the caller's saved default, then to the system one. */
  template_id?: string;

  /** Pre-fills the CV from the stored résumé structure. */
  seed?: boolean;
}

model CvUpdate {
  title?: string;
  template_id?: string;
  document?: unknown;
}

model CvTemplateChoice {
  template_id?: string;
}

model CvTracerLinksSetting {
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
  session_id?: string;
}

model CvBaseSession {
  base_cv_id: string;
  session_id: string;
}

model CvTailorRequest {
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
@tag("CV")
@route("/cv-templates")
@get
op listCVTemplates(): Data<unknown> | Problem;

/** The registered typefaces a CV may choose. */
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
@tag("CV")
@route("/me/cvs")
@get
op listCVs(): Data<CvTailored[]> | Problem;

/** The base CVs, oldest first, with the default flagged. With a session or an API key. */
@tag("CV")
@route("/me/cvs/bases")
@get
op listBaseCVs(): Data<CvBase[]> | Problem;

@tag("CV")
@route("/me/cvs")
@post
op createCV(@body body: CvCreate): {
  @statusCode _: 201;
  @body body: Data<CvRecord>;
} | Problem;

/** The document without its contact block to an API key. */
@tag("CV")
@route("/me/cvs/{id}")
@get
op getCV(@path id: string): Data<CvRecord> | Problem;

@tag("CV")
@route("/me/cvs/{id}")
@put
op updateCV(@path id: string, @body body: CvUpdate): Data<CvMeta> | Problem;

@tag("CV")
@route("/me/cvs/{id}")
@delete
op deleteCV(@path id: string): NoContentResponse | Problem;

@tag("CV")
@route("/me/cvs/{id}/template")
@put
op setCVTemplate(@path id: string, @body body: CvTemplateChoice): NoContentResponse | Problem;

/** 409 when enabling on a deployment with no visitor salt. */
@tag("CV")
@route("/me/cvs/{id}/tracer-links")
@put
op setCVTracerLinks(@path id: string, @body body: CvTracerLinksSetting): Data<CvTracerLinksState> | Problem;

@tag("CV")
@route("/me/cvs/{id}/tracer-links")
@get
op listCVTracerLinks(@path id: string): Data<CvTracerLink[]> | Problem;

/** Copies a base CV as "<title> (copy)"; 409 for a tailored one. */
@tag("CV")
@route("/me/cvs/{id}/duplicate")
@post
op duplicateCV(@path id: string): {
  @statusCode _: 201;
  @body body: Data<CvRecord>;
} | Problem;

/** The assistant conversation for a base CV; not behind the paid gate. */
@tag("CV")
@route("/me/cvs/{id}/base-session")
@post
op startBaseSession(@path id: string): Data<CvBaseSession> | Problem;

/**
 * Starts the tailoring session for a vacancy (the paid gate runs before this). 201 with
 * the ids and the cached analysis; 402 when the allowance is spent and the vacancy is new.
 * With a session or an API key.
 */
@tag("CV")
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

@tag("CV")
@route("/me/cvs/{id}/tailor-session")
@post
op startTailorSession(@path id: string): {
  @statusCode _: 201;
  @body body: Data<CvTailorSession>;
} | Problem;

/** Rebuilds the default base CV from the current seed. */
@tag("CV")
@route("/me/cvs/base/reseed")
@post
op reseedBaseCV(): Data<CvRecord> | Problem;

/** Rebuilds one CV from the current seed; destructive. */
@tag("CV")
@route("/me/cvs/{id}/reseed")
@post
op reseedCV(@path id: string): Data<CvRecord> | Problem;

/**
 * Applies a batch of path operations (`{"ops": [...], "note": ""}`), decoded strictly in
 * Go: an unknown field or bad addressing is a 422. With a session or an API key.
 */
@tag("CV")
@route("/me/cvs/{id}")
@patch
op patchCV(@path id: string, @body body: unknown): Data<CvMeta> | Problem;

@tag("CV")
@route("/me/cvs/{id}/session")
@put
op setCVSession(@path id: string, @body body: CvSessionBinding): NoContentResponse | Problem;

/** The cached fit analysis of a tailored CV, projected for the tailoring agent. */
@tag("CV")
@route("/me/cvs/{id}/tailor-context")
@get
op getCVTailorContext(@path id: string): Data<unknown> | Problem;

/** Never calls a model. With a session or an API key. */
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

/** What changed the CV, newest first. */
@tag("CV")
@route("/me/cvs/{id}/revisions")
@get
op listCVRevisions(@path id: string): Data<unknown> | Problem;

@tag("CV")
@route("/me/cvs/{id}/revisions/{rid}/undo")
@post
op undoCVRevision(@path id: string, @path rid: string): Data<CvRevisionUndo> | Problem;

@tag("CV")
@route("/me/cvs/{id}/revisions/batch/{bid}/undo")
@post
op undoCVRevisionBatch(@path id: string, @path bid: string): Data<CvMeta> | Problem;

@tag("CV")
@route("/me/cvs/{id}/ats-delta")
@get
op getCVATSDelta(@path id: string): Data<CvAtsDelta> | Problem;

@tag("CV")
@route("/me/cvs/{id}/job-match")
@get
op getCVJobMatch(@path id: string): Data<CvJobMatch> | Problem;
