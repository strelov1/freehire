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
