import type { Http } from '../api-core';
import type { GhostReportInput, ProcessReportKind, ProcessReportResult, Report, ReportInput } from '../types';
import { jsonBody } from '../api-core';
import type { components } from '#lib/generated/api.js';

export function reportsApi({ call, requestData }: Http) {
  /** Report a problem with a live vacancy (by slug). Returns the pending report. */
  async function reportJob(slug: string, input: ReportInput): Promise<Report> {
    return requestData<Report>(
      `/api/v1/jobs/${encodeURIComponent(slug)}/reports`,
      jsonBody('POST', input satisfies components['schemas']['JobReportRequest']),
    );
  }

  /** File a ghost-signal claim: applied on this date, never answered. Unlike
   *  reportJob this reaches no moderator and closes nothing — it accumulates as
   *  evidence, and `withdrawGhostReport` takes it back when an employer answers. */
  async function reportGhostJob(slug: string, input: GhostReportInput): Promise<void> {
    await call(
      `/api/v1/jobs/${slug}/ghost-report`,
      jsonBody('POST', input satisfies components['schemas']['GhostReportRequest']),
    );
  }

  /** Report a fact about how a COMPANY hires — today only that it screens with an AI
   *  interviewer. Filed from a job page but addressed by company, because that is what
   *  the report is about. Reaches no moderator: there is nothing to act on about a true
   *  statement. Returns the company's resulting count, so the caller can render the
   *  badge from this answer rather than re-reading the company. 409 if already filed. */
  async function reportCompanyProcess(slug: string, kind: ProcessReportKind): Promise<ProcessReportResult> {
    return requestData<ProcessReportResult>(
      `/api/v1/companies/${encodeURIComponent(slug)}/process-reports`,
      jsonBody('POST', { kind } satisfies components['schemas']['ProcessReportRequest']),
    );
  }

  /** Withdraw this caller's own process report (404 if there is none). The row is kept
   *  and stamped rather than deleted, so withdrawing cannot be used to file twice. */
  async function withdrawCompanyProcessReport(slug: string, kind: ProcessReportKind): Promise<ProcessReportResult> {
    return requestData<ProcessReportResult>(
      `/api/v1/companies/${encodeURIComponent(slug)}/process-reports?kind=${encodeURIComponent(kind)}`,
      { method: 'DELETE' },
    );
  }

  /** Which process-report kinds this caller currently holds against a company, so the
   *  write surface opens in the right state instead of finding out by being refused. */
  async function myCompanyProcessReports(slug: string): Promise<ProcessReportKind[]> {
    return requestData<ProcessReportKind[]>(`/api/v1/companies/${encodeURIComponent(slug)}/process-reports/mine`);
  }

  /** Withdraw this caller's ghost claim about a job (204, or 404 if there is none). */
  async function withdrawGhostReport(slug: string): Promise<void> {
    // call(), not request(): a 204 carries no body, and .json() on an empty one
    // throws on a call that in fact succeeded — the mirror of why the endpoint
    // answers 204 rather than a 200 whose body would read "OK".
    await call(`/api/v1/jobs/${slug}/ghost-report`, { method: 'DELETE' });
  }

  /** The moderator review queue: pending reports, with reporter email and job fields. */
  async function listPendingReports(): Promise<Report[]> {
    return requestData<Report[]>('/api/v1/reports');
  }

  /** Resolve a pending report; optionally soft-close the reported job. The note is
   *  emailed to the reporter when `notifyReporter` is set. */
  async function resolveReport(id: number, closeJob: boolean, note = '', notifyReporter = false): Promise<Report> {
    return requestData<Report>(
      `/api/v1/reports/${id}/resolve`,
      jsonBody('POST', { close_job: closeJob, note, notify_reporter: notifyReporter }),
    );
  }

  /** Dismiss a pending report with an optional reason; the job is unchanged. The reason
   *  is emailed to the reporter when `notifyReporter` is set. */
  async function dismissReport(id: number, reason = '', notifyReporter = false): Promise<Report> {
    return requestData<Report>(
      `/api/v1/reports/${id}/dismiss`,
      jsonBody('POST', { reason, notify_reporter: notifyReporter }),
    );
  }

  return {
    reportJob,
    reportGhostJob,
    reportCompanyProcess,
    withdrawCompanyProcessReport,
    myCompanyProcessReports,
    withdrawGhostReport,
    listPendingReports,
    resolveReport,
    dismissReport,
  };
}
