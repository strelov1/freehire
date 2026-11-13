import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// Job reports: a signed-in person (cookie or API key) flags a problem with a live
// vacancy, and moderators review them. Resolving may soft-close the reported job.

/** The closed reason vocabulary (internal/engage/report validReasons and the CHECK on job_reports). */
enum JobReportReason {
  no_response,
  not_relevant,
  spam,
  fraud,
  other,
}

enum JobReportStatus {
  pending,
  resolved,
  dismissed,
}

model JobReport {
  id: int64;
  reason: JobReportReason;
  details: string;
  contact_telegram?: string;
  status: JobReportStatus;
  review_reason?: string;
  reviewed_at?: utcDateTime;
  created_at: utcDateTime | null;

  /** Only on the moderator queue. */
  reporter_email?: string;

  job_slug?: string;
  job_title?: string;

  /** Only on a decision: whether the reporter was actually told. */
  notified?: boolean;
}

model JobReportRequest {
  /** From the controlled vocabulary. */
  reason?: string;

  /** Free-text details. */
  details?: string;
  /** Optional contact handle. */
  contact_telegram?: string;
}

model ResolveJobReportRequest {
  close_job?: boolean;
  note?: string;
  notify_reporter?: boolean;
}

model DismissJobReportRequest {
  reason?: string;
  notify_reporter?: boolean;
}

/** 404 for an unknown job, 409 for a second open report by the same person. */
@summary("Report a problem with a job.")
@useAuth(SessionOrKey)
@tag("Reports")
@route("/jobs/{slug}/reports")
@post
op createJobReport(/** The job `public_slug`. */ @path slug: string, @body body: JobReportRequest): {
  @statusCode _: 201;
  @body body: Data<JobReport>;
} | Problem;

/** Moderators only. */
@summary("The pending report queue (moderators).")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Reports")
@route("/reports")
@get
op listPendingJobReports(): Data<JobReport[]> | Problem;

/** Moderators only. The body is optional. 404 for an unknown id, 409 for one already decided. */
@summary("Resolve a report, optionally closing the job and telling the reporter.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Reports")
@route("/reports/{id}/resolve")
@post
op resolveJobReport(/** The report id. (e.g. `3`) */ @path id: string, @body body?: ResolveJobReportRequest): Data<JobReport> | Problem;

/** Moderators only. The body is optional. */
@summary("Dismiss a report with a reason, optionally telling the reporter.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Reports")
@route("/reports/{id}/dismiss")
@post
op dismissJobReport(/** The report id. (e.g. `3`) */ @path id: string, @body body?: DismissJobReportRequest): Data<JobReport> | Problem;
