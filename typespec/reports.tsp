import "@typespec/http";

using Http;

namespace Freehire;

// Job reports: a signed-in person (cookie or API key) flags a problem with a live
// vacancy, and moderators review them. Resolving may soft-close the reported job.

model JobReport {
  id: int64;
  reason: string;
  details: string;
  contact_telegram?: string;
  status: string;
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

  details?: string;
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
@tag("Reports")
@route("/jobs/{slug}/reports")
@post
op createJobReport(@path slug: string, @body body: JobReportRequest): {
  @statusCode _: 201;
  @body body: Data<JobReport>;
} | Problem;

/** Moderators only. */
@tag("Reports")
@route("/reports")
@get
op listPendingJobReports(): Data<JobReport[]> | Problem;

/** Moderators only. The body is optional. 404 for an unknown id, 409 for one already decided. */
@tag("Reports")
@route("/reports/{id}/resolve")
@post
op resolveJobReport(@path id: string, @body body?: ResolveJobReportRequest): Data<JobReport> | Problem;

/** Moderators only. The body is optional. */
@tag("Reports")
@route("/reports/{id}/dismiss")
@post
op dismissJobReport(@path id: string, @body body?: DismissJobReportRequest): Data<JobReport> | Problem;
