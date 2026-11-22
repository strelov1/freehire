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

  /** True when the caller acts for the company as a member of its team rather than as its claimant; `work_email` is then the caller's own. */
  member: boolean;

  /** The exact values are owner for the claimant, manager for an existing manager, and recruiter. */
  role?: string;

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

enum EmployerClaimReviewReason {
  unknown_website,
  domain_mismatch,
  academic_domain,
  manual_review,
}

/** A pending claim with the current domain check, not a stored historical failure reason. */
model EmployerPendingClaim {
  ...EmployerAccount;
  /** The catalogue's current website, empty when unknown. */
  website: string;
  /** manual_review means the domains now match but the claim still awaits a moderator. */
  review_reason: EmployerClaimReviewReason;
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
op listEmployerPendingClaims(): Data<EmployerPendingClaim[]> | Problem;

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
// hiring pipeline over them. Only delivered applications are listed. Every stage shows in
// the candidate's own tracking (new as applied, reviewing as screening, hired as
// accepted), and `interview`, `offer` and `rejected` email the candidate (Reply-To the
// account's work email).

/** A stage of the employer's hiring pipeline. Every delivered application starts at `new`. */
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
  /** The question's id on the vacancy's form. (e.g. `q1`) */
  id: string;

  /** The question as it read when the candidate applied. */
  label: string;

  /** "text", "boolean" (answer "true"/"false") or "select". */
  type: string;

  /** The candidate's answer. */
  answer: string;
}

/** One application sent on freehire to one of the caller's vacancies. */
model EmployerApplication {
  id: int64;

  /** The vacancy's public slug. */
  job_slug: string;

  job_title: string;

  /** The candidate's address; replies to the application email reach it too. */
  candidate_email: string;

  /** "tailored" (a CV made for this vacancy) or "resume" (the candidate's uploaded file). */
  cv_kind: string;

  /** False for an application sent before its CV was kept: that CV reached you by email only. */
  has_cv: boolean;

  /** The candidate's note, empty when none. */
  note: string;

  answers: EmployerApplicationAnswer[];
  stage: EmployerApplicationStage;

  /** When you last moved it; absent while it has never been moved. */
  stage_changed_at?: utcDateTime;

  /** First review by a teammate, independent of the hiring stage. */
  viewed_at?: utcDateTime;

  /** Private notes shared within the employer team. */
  employer_notes?: string;

  /** When the application reached you. */
  sent_at: utcDateTime;
}

model EmployerApplicationsMeta {
  ...PageMeta;

  /** How many applications sit at each stage under the `job` filter, whatever `stage` is; a stage with none is absent. */
  stage_counts: Record<int64>;

  /** Query params this listing did not read, so the answer is wider than asked; absent when every param was read. */
  ignored_params?: IgnoredParam[];
}

model EmployerApplicationsPage {
  data: EmployerApplication[];
  meta: EmployerApplicationsMeta;
}

model EmployerApplicationMove {
  /** The stage to move to: new, reviewing, interview, offer, hired or rejected. (e.g. `interview`) */
  stage?: string;

  /** Your words to the candidate, at most 2000 characters, quoted in their email. Accepted only with `interview`, `offer` or `rejected`, the stages that send one. */
  message?: string;
}

model EmployerApplicationMoved {
  application: EmployerApplication;

  /** Whether the candidate was emailed about this move. False when the stage sends no letter, nothing changed, or the letter failed. */
  candidate_notified: boolean;
}

/**
 * The delivered applications to the caller's vacancies, newest first. An unknown `stage` is
 * a 400 rather than an unfiltered answer; a param the listing does not read is named in
 * `meta.ignored_params`.
 */
@summary("List the applications sent to your vacancies.")
@tag("Employer")
@opExample(#{
  parameters: #{ stage: "new" },
  returnType: #{
    data: #[
      #{
        id: 42,
        job_slug: "platform-engineer-acme-9f3c",
        job_title: "Platform Engineer",
        candidate_email: "candidate@example.com",
        cv_kind: "tailored",
        has_cv: true,
        note: "Happy to talk this week.",
        answers: #[#{ id: "q1", label: "EU work permit?", type: "boolean", answer: "true" }],
        stage: EmployerApplicationStage.new,
        sent_at: utcDateTime.fromISO("2026-11-10T10:00:00Z"),
      },
    ],
    meta: #{ total: 1, limit: 50, offset: 0, stage_counts: #{ new: 1, rejected: 2 } },
  },
})
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

model EmployerApplicationSearch {
  /** A literal search in candidate email, message and answers. */
  @maxLength(200)
  query: string;
}

/** Search delivered applications before pagination. An unknown stage returns 400. */
@summary("Search applications sent to your vacancies.")
@tag("Employer")
@route("/employer/applications/search")
@post
op searchEmployerApplications(
  /** Only applications for this vacancy slug. */
  @query job?: string,
  /** Only applications at this stage. */
  @query stage?: string,
  /** Maximum applications to return. */
  @query limit?: string,
  /** Number of matching applications to skip. */
  @query offset?: string,
  @body body: EmployerApplicationSearch,
): EmployerApplicationsPage | Problem;

model EmployerApplicationWorkspace {
  /** Record that a teammate reviewed this application. */
  mark_viewed?: boolean;

  /** Replace the private team notes. The empty string clears them. */
  @maxLength(4000)
  employer_notes?: string;
}

@summary("Mark an application reviewed or save private notes.")
@tag("Employer")
@route("/employer/applications/{id}/workspace")
@patch
op updateEmployerApplicationWorkspace(
  @path id: string,
  @body body: EmployerApplicationWorkspace,
): Data<EmployerApplication> | Problem;

/** The CV file the application sent, exactly as it was emailed to you: a PDF, or the text the candidate pasted. 404 when none was kept. */
@summary("Open the CV an application sent.")
@tag("Employer")
@opExample(#{ parameters: #{ id: "42" } })
@route("/employer/applications/{id}/cv")
@get
op getEmployerApplicationCV(
  /** The application's id. (e.g. `42`) */
  @path id: string,
): {
  @header contentType: string;
  @header("Content-Disposition") contentDisposition: string;
  @header("Cache-Control") cacheControl: string;
  @body cv: bytes;
} | Problem;

/**
 * Moves the application to a stage of your pipeline. The candidate's tracking follows, and
 * `interview`, `offer` and `rejected` email the candidate with your optional message. A move
 * to the stage it already holds changes nothing and sends nothing.
 */
@summary("Move an application through your pipeline.")
@tag("Employer")
@opExample(#{
  parameters: #{ id: "42", body: #{ stage: "interview", message: "Are you free on Tuesday?" } },
  returnType: #{
    data: #{
      application: #{
        id: 42,
        job_slug: "platform-engineer-acme-9f3c",
        job_title: "Platform Engineer",
        candidate_email: "candidate@example.com",
        cv_kind: "tailored",
        has_cv: true,
        note: "",
        answers: #[],
        stage: EmployerApplicationStage.interview,
        stage_changed_at: utcDateTime.fromISO("2026-11-12T09:00:00Z"),
        sent_at: utcDateTime.fromISO("2026-11-10T10:00:00Z"),
      },
      candidate_notified: true,
    },
  },
})
@route("/employer/applications/{id}/stage")
@post
op moveEmployerApplication(
  /** The application's id. (e.g. `42`) */
  @path id: string,
  @body body: EmployerApplicationMove,
): Data<EmployerApplicationMoved> | Problem;

// ── The company team ────────────────────────────────────────────────────────────────
// Colleagues join by a single-use invitation. New invitations grant recruiter access.
// The company owns its vacancies and applications.

/** Someone who acts for the company. `holder` is the claimant, who cannot be removed. */
model EmployerTeamMember {
  user_id: int64;
  work_email: string;
  joined_at: utcDateTime;
  holder: boolean;
  role?: string;
}

/** An invitation whose link still works. */
model EmployerTeamInvitation {
  id: int64;
  email: string;
  created_at: utcDateTime;
  expires_at: utcDateTime;
}

model EmployerTeam {
  members: EmployerTeamMember[];
  invitations: EmployerTeamInvitation[];
}

model EmployerTeamInvite {
  email?: string;
}

/** The token from the invitation link. */
model EmployerTeamToken {
  token?: string;
}

/** What an invitation link offers before it is accepted. */
model EmployerTeamInvitationPreview {
  company_name: string;
  email: string;
  role?: string;
}

model EmployerRemoved {
  removed: boolean;
}

/** The company team and open invitations. A recruiter sees their own membership. */
@tag("Employer")
@route("/employer/team")
@get
op getEmployerTeam(): Data<EmployerTeam> | Problem;

/** Mails a colleague a single-use recruiter invitation, valid seven days. Inviting an address again replaces its link. 429 past twenty open invitations. */
@tag("Employer")
@route("/employer/team/invitations")
@post
op inviteEmployerTeamMember(@body body: EmployerTeamInvite): {
  @statusCode _: 201;
  @body body: Data<EmployerTeamInvitation>;
} | Problem;

/** Withdraws an invitation; its link stops working. */
@tag("Employer")
@route("/employer/team/invitations/{id}")
@delete
op withdrawEmployerTeamInvitation(@path id: string): Data<EmployerRemoved> | Problem;

/** Removes a member, or the caller themselves (leaving). Their vacancies stay with the team. The claimant cannot be removed. */
@tag("Employer")
@route("/employer/team/members/{user_id}")
@delete
op removeEmployerTeamMember(@path user_id: string): Data<EmployerRemoved> | Problem;

/** Which company an invitation link invites to, at which address. 404 for a dead link. */
@tag("Employer")
@route("/employer/team/invitation-preview")
@post
op previewEmployerTeamInvitation(@body body: EmployerTeamToken): Data<EmployerTeamInvitationPreview> | Problem;

/** Joins the team. A recruiter must sign in with the matching verified email. 409 when the caller already acts for a company. */
@tag("Employer")
@route("/employer/team/join")
@post
op joinEmployerTeam(@body body: EmployerTeamToken): Data<EmployerAccount> | Problem;


model EmployerActivity {
  id: int64;
  action: string;
  actor_email?: string;
  application_id?: int64;
  previous_stage?: string;
  stage?: string;
  subject_email?: string;
  occurred_at: utcDateTime;
}

/** Up to 30 company audit events, newest first. Recruiters see application activity. */
@tag("Employer")
@route("/employer/activity")
@get
op listEmployerActivity(
  /** Fetch older events by passing the last event id from the previous page. */
  @query before?: string,
): Data<EmployerActivity[]> | Problem;
