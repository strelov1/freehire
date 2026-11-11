import "@typespec/http";

using Http;

namespace Freehire;

model DirectApplyQuestion {
  id: string;
  label: string;
  type: string;
  required: boolean;
  options?: string[];
}

model DirectApplyTailoredCV {
  id: string;
  title: string;
}

/** What the signed-in candidate can send. */
model DirectApplyCVOptions {
  /** The CV tailored for this vacancy, null when none. */
  tailored: DirectApplyTailoredCV | null;

  has_resume: boolean;
}

model DirectApplyForm {
  company_name: string;
  questions: DirectApplyQuestion[];
  open: boolean;
  applied: boolean;
  prefill: Record<string>;
  cvs: DirectApplyCVOptions;
}

/** Names the CV to send: `resume: true` for the uploaded one, or `cv_id`, exactly one. */
model DirectApplyRequest {
  resume?: boolean;
  cv_id?: string;
  answers?: Record<string>;
  note?: string;
}

model DirectApplySent {
  sent: boolean;
}

/** The vacancy's questions and, for a signed-in candidate, their prefill. 501 when the deployment cannot deliver applications. */
@tag("DirectApply")
@route("/jobs/{slug}/direct-apply")
@get
op getDirectApplyForm(@path slug: string): Data<DirectApplyForm> | Problem;

/** Sends the caller's application. Cookie session only. */
@tag("DirectApply")
@route("/jobs/{slug}/direct-apply")
@post
op sendDirectApply(@path slug: string, @body body: DirectApplyRequest): {
  @statusCode _: 201;
  @body body: Data<DirectApplySent>;
} | Problem;
