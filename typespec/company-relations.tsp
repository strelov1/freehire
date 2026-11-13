import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// Corrections people propose about two companies (the same employer, or one the
// subsidiary of another) and the moderator queue that decides them.

/** `same`: one employer under two names; `subsidiary`: the page's company is owned by the target. */
enum CompanyRelationKind {
  same,
  subsidiary,
}

/** `approved` marks a merge whose job move stopped part-way; approving again resumes it. */
enum PendingCompanyRelationStatus {
  pending,
  approved,
}

model CompanyRelationRequest {
  /** `same` or `subsidiary`. */
  kind?: string;

  target_slug?: string;
  note?: string;
}

model CompanyRelationProposal {
  id: int64;
  kind: string;
  company_slug: string;
  target_slug: string;
  note: string;
  status: string;
  created_at: utcDateTime;
}

model PendingCompanyRelation {
  id: int64;
  kind: CompanyRelationKind;
  company_slug: string;
  company_name: string;
  company_job_count: int64;
  target_slug: string;
  target_name: string;
  target_job_count: int64;
  note: string;
  created_at: utcDateTime;

  status: PendingCompanyRelationStatus;
}

model CompanyRelationApproval {
  kind: string;

  /** Jobs a `same` approval re-keyed; 0 for a subsidiary. */
  moved_jobs: int64;
}

model CompanyRelationRejection {
  rejected: boolean;
}

/** Session cookie only. The path's slug is the company that retires, or the subsidiary. */
@tag("Company relations")
@route("/companies/{slug}/relations")
@post
op proposeCompanyRelation(@path slug: string, @body body: CompanyRelationRequest): {
  @statusCode _: 201;
  @body body: Data<CompanyRelationProposal>;
} | Problem;

/** Moderators only, oldest first. */
@extension("x-audience", "moderator")
@tag("Company relations")
@route("/company-relations")
@get
op listPendingCompanyRelations(): Data<PendingCompanyRelation[]> | Problem;

/** Moderators only. Approving again resumes a move that failed part-way. */
@extension("x-audience", "moderator")
@tag("Company relations")
@route("/company-relations/{id}/approve")
@post
op approveCompanyRelation(@path id: string): Data<CompanyRelationApproval> | Problem;

/** Moderators only. */
@extension("x-audience", "moderator")
@tag("Company relations")
@route("/company-relations/{id}/reject")
@post
op rejectCompanyRelation(@path id: string): Data<CompanyRelationRejection> | Problem;
