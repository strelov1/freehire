import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// The verified-employer surface (session cookie only): claiming a company, editing its
// curated profile, publishing and closing its own vacancies, and the moderator/admin
// review of a claim. A vacancy is the shared job view (internal/job/jobview), whose wide
// shape belongs to search too, so it travels as an opaque value.

enum EmployerAccountStatus {
  pending,
  active,
  revoked,
}

/** A company account. `user_id` is the address the moderator and admin actions take. */
model EmployerAccount {
  user_id: int64;
  company_slug: string;
  company_name: string;
  work_email: string;
  status: EmployerAccountStatus;
  verified_at?: utcDateTime;

  /** Whether the claimant still owes the mailed code or is waiting on a moderator. */
  email_confirmed: boolean;

  created_at: utcDateTime;
}

/** The account plus its company's curated profile; an unset profile field is left out. */
model EmployerCompany {
  ...EmployerAccount;
  tagline?: string;
  description?: string;
  website?: string;
  industries?: string[];
  year_founded?: int64;
  employee_count?: int64;
  hq_country?: string;
  subindustry?: string;
}

model EmployerClaim {
  company_name?: string;
  work_email?: string;
}

model EmployerConfirm {
  code?: string;
}

/** An absent or null field is kept; `industries` replaces the whole curated set, even as `[]`. */
model EmployerCompanyPatch {
  tagline?: string | null;
  description?: string | null;
  website?: string | null;
  industries?: string[] | null;
  year_founded?: int64 | null;
  employee_count?: int64 | null;
  hq_country?: string | null;
  subindustry?: string | null;
}

/** `type` is "text", "boolean" or "select"; `options` are a select's choices. */
model EmployerQuestion {
  label?: string;
  type?: string;
  required?: boolean;
  options?: string[];
}

/** `title` is required, and `url` too unless `apply_mode` is "freehire". */
model EmployerVacancyCreate {
  apply_mode?: string;
  questions?: EmployerQuestion[];
  url?: string;
  title?: string;
  location?: string;
  remote?: boolean;
  description?: string;
  posted_at?: utcDateTime | null;
  skills?: string[];
  regions?: string[];
  cities?: string[];
  work_mode?: string;
  employment_type?: string;
  seniority?: string;
  salary_min?: int64 | null;
  salary_max?: int64 | null;
  salary_currency?: string;
  salary_period?: string;
}

/** An absent or null field is kept. */
model EmployerVacancyPatch {
  apply_mode?: string | null;
  questions?: EmployerQuestion[] | null;
  title?: string | null;
  location?: string | null;
  remote?: boolean | null;
  description?: string | null;
  posted_at?: utcDateTime | null;
}

model EmployerClosed {
  closed: boolean;
}

model EmployerRejected {
  rejected: boolean;
}

/** Starts a claim: reserves the company and mails a code to the work email. */
@tag("Employer")
@route("/employer/claim")
@post
op claimEmployerCompany(@body body: EmployerClaim): {
  @statusCode _: 201;
  @body body: Data<EmployerAccount>;
} | Problem;

/** Activates at once when the work email matches the company's website, else stays pending. */
@tag("Employer")
@route("/employer/claim/confirm")
@post
op confirmEmployerClaim(@body body: EmployerConfirm): Data<EmployerAccount> | Problem;

/** 409 without a pending claim or when the email is confirmed; 429 inside the cooldown. */
@tag("Employer")
@route("/employer/claim/resend")
@post
op resendEmployerClaimCode(): Data<EmployerAccount> | Problem;

/** The caller's own account in any status, with the company profile. */
@tag("Employer")
@route("/employer/company")
@get
op getEmployerCompany(): Data<EmployerCompany> | Problem;

@tag("Employer")
@route("/employer/company")
@patch
op updateEmployerCompany(@body body: EmployerCompanyPatch): Data<EmployerCompany> | Problem;

/** Every vacancy the caller published, newest first. */
@tag("Employer")
@route("/employer/jobs")
@get
op listEmployerVacancies(): Data<unknown[]> | Problem;

/** Publishes, or idempotently updates the caller's own vacancy at the same URL. */
@tag("Employer")
@route("/employer/jobs")
@post
op createEmployerVacancy(@body body: EmployerVacancyCreate): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

@tag("Employer")
@route("/employer/jobs/{slug}")
@patch
op updateEmployerVacancy(@path slug: string, @body body: EmployerVacancyPatch): Data<unknown> | Problem;

@tag("Employer")
@route("/employer/jobs/{slug}/close")
@post
op closeEmployerVacancy(@path slug: string): Data<EmployerClosed> | Problem;

@tag("Employer")
@route("/employer/jobs/{slug}/reopen")
@post
op reopenEmployerVacancy(@path slug: string): Data<EmployerClosed> | Problem;

/** Moderators only: the claims that could not auto-activate. */
@extension("x-audience", "moderator")
@tag("Employer")
@route("/employer/claims")
@get
op listEmployerPendingClaims(): Data<EmployerAccount[]> | Problem;

/** Moderators only. The user id travels as a string so a non-number keeps its 400. */
@extension("x-audience", "moderator")
@tag("Employer")
@route("/employer/claims/{user_id}/approve")
@post
op approveEmployerClaim(@path user_id: string): Data<EmployerAccount> | Problem;

/** Moderators only. */
@extension("x-audience", "moderator")
@tag("Employer")
@route("/employer/claims/{user_id}/reject")
@post
op rejectEmployerClaim(@path user_id: string): Data<EmployerRejected> | Problem;

/** Admins only: deactivates the account but keeps the company reservation. */
@tag("Employer")
@route("/employer/claims/{user_id}/revoke")
@post
op revokeEmployerAccount(@path user_id: string): Data<EmployerAccount> | Problem;

// ── The applicant inbox ─────────────────────────────────────────────────────────────
// The applications sent on freehire to the caller's own vacancies, and the caller's
// hiring pipeline over them. Only delivered applications are listed. Every stage but
// `new` shows in the candidate's own tracking, and `interview`, `offer` and `rejected`
// email the candidate (Reply-To the account's work email).

enum EmployerApplicationStage {
  new,
  reviewing,
  interview,
  offer,
  hired,
  rejected,
}

/** One answered question, as asked when the candidate applied. */
model EmployerApplicationAnswer {
  id: string;
  label: string;

  /** "text", "boolean" (answer "true"/"false") or "select". */
  type: string;

  answer: string;
}

/** One application sent on freehire to one of the caller's vacancies. */
model EmployerApplication {
  id: int64;
  job_slug: string;
  job_title: string;
  candidate_email: string;

  /** "tailored" (a CV made for this vacancy) or "resume" (the candidate's uploaded file). */
  cv_kind: string;

  /** False for an application sent before its CV was kept: that CV reached you by email only. */
  has_cv: boolean;

  note: string;
  answers: EmployerApplicationAnswer[];
  stage: EmployerApplicationStage;

  /** When you last moved it; absent while it has never been moved. */
  stage_changed_at?: utcDateTime;

  sent_at: utcDateTime;
}

model EmployerApplicationsMeta {
  ...PageMeta;

  /** How many applications sit at each stage under the `job` filter, whatever `stage` is; a stage with none is absent. */
  stage_counts: Record<int64>;
}

model EmployerApplicationsPage {
  data: EmployerApplication[];
  meta: EmployerApplicationsMeta;
}

/** `message` is optional and goes only with `interview`, `offer` or `rejected`. */
model EmployerApplicationMove {
  stage?: string;
  message?: string;
}

model EmployerApplicationMoved {
  application: EmployerApplication;

  /** Whether the candidate was emailed about this move. False when the stage sends no letter, nothing changed, or the letter failed. */
  candidate_notified: boolean;
}

/** The delivered applications to the caller's vacancies, newest first. */
@tag("Employer")
@route("/employer/applications")
@get
op listEmployerApplications(
  /** Only this vacancy's applications. (e.g. `platform-engineer-acme-9f3c`) */
  @query job?: string,
  /** Only applications at this stage. (e.g. `new`) */
  @query stage?: string,
  /** Page size, 1–100. (e.g. `50`) */
  @query limit?: string,
  /** Rows to skip. (e.g. `0`) */
  @query offset?: string,
): EmployerApplicationsPage | Problem;

/** The CV file the application sent, exactly as it was emailed to you. 404 when none was kept. */
@tag("Employer")
@route("/employer/applications/{id}/cv")
@get
op getEmployerApplicationCV(@path id: string): {
  @header contentType: string;
  @header("Content-Disposition") contentDisposition: string;
  @header("Cache-Control") cacheControl: string;
  @body cv: bytes;
} | Problem;

/** Moves the application to a stage of your pipeline. */
@tag("Employer")
@route("/employer/applications/{id}/stage")
@post
op moveEmployerApplication(@path id: string, @body body: EmployerApplicationMove): Data<EmployerApplicationMoved> | Problem;
