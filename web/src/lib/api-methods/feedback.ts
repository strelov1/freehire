import type { Http } from '../api-core';
import type { CompanyFeedback, CompanyFeedbackSummary, ReportedCompanyFeedback } from '../types';
import type { Page, Slice } from '../api-types';
import { jsonBody, toSlice } from '../api-core';

export function feedbackApi({ call, request, requestData }: Http) {
  /** A company's feedback, newest first, offset-paginated. Public. */
  async function listCompanyFeedback(slug: string, limit: number, offset: number): Promise<Slice<CompanyFeedback>> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return toSlice(await request<Page<CompanyFeedback>>(`/api/v1/companies/${slug}/feedback?${params}`), offset);
  }

  /** All of the caller's own feedback on a company, across every category
   *  they've reviewed it under (empty when they have not left any yet) — the
   *  write form's prefill and its "which categories are already taken" read.
   *  Requires a session. */
  function getMyCompanyFeedback(slug: string): Promise<CompanyFeedback[]> {
    return requestData<CompanyFeedback[]>(`/api/v1/companies/${slug}/feedback/mine`);
  }

  /** Create or overwrite the caller's feedback in one category on a company in
   *  place (edit-by-resubmit within that category — a different category is a
   *  separate review, not an overwrite). Requires a session. `company` is
   *  always present here (unlike the shared CompanyFeedback shape, where it's
   *  List/Mine's unused optional field) — the freshly recomputed counters, so
   *  the caller can update its own view of the company without a second round
   *  trip. */
  function upsertCompanyFeedback(
    slug: string,
    input: { rating: number; feedback_type: string; body: string },
  ): Promise<CompanyFeedback & { company: CompanyFeedbackSummary }> {
    return requestData<CompanyFeedback & { company: CompanyFeedbackSummary }>(
      `/api/v1/companies/${slug}/feedback`,
      jsonBody('POST', input),
    );
  }

  /** Delete the caller's own feedback in one category on a company (idempotent
   *  — a no-op when none exists still succeeds), returning the company's
   *  freshly recomputed counters so the caller never needs a follow-up fetch
   *  to learn the new count. */
  function deleteCompanyFeedback(slug: string, feedbackType: string): Promise<CompanyFeedbackSummary> {
    const params = new URLSearchParams({ feedback_type: feedbackType });
    return requestData<CompanyFeedbackSummary>(`/api/v1/companies/${slug}/feedback?${params}`, { method: 'DELETE' });
  }

  /** Flag a specific review as spam/offensive/false/other — evidence for a
   *  moderator, not a full report ticket (see reportJob for that fuller
   *  shape). A second report of the same review by the same caller is a
   *  silent no-op. Requires a session. */
  async function reportCompanyFeedback(feedbackId: number, reason: string): Promise<void> {
    await call(`/api/v1/company-feedback/${feedbackId}/report`, jsonBody('POST', { reason }));
  }

  /** The moderator queue: every review with at least one report, most-reported
   *  first. Role-gated. */
  function listReportedCompanyFeedback(): Promise<ReportedCompanyFeedback[]> {
    return requestData<ReportedCompanyFeedback[]>('/api/v1/company-feedback/reported');
  }

  /** Hide a specific review, dropping it out of its company's public list and
   *  average immediately. Idempotent. Role-gated. */
  async function hideCompanyFeedback(feedbackId: number): Promise<void> {
    await call(`/api/v1/company-feedback/${feedbackId}/hide`, { method: 'POST' });
  }

  return {
    listCompanyFeedback,
    getMyCompanyFeedback,
    upsertCompanyFeedback,
    deleteCompanyFeedback,
    reportCompanyFeedback,
    listReportedCompanyFeedback,
    hideCompanyFeedback,
  };
}
