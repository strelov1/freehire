import type { Http } from '../api-core';
import type {
  EmployerAccount,
  EmployerPendingClaim,
  EmployerApplication,
  EmployerApplicationMoved,
  EmployerApplicationsPage,
  EmployerApplicationStage,
  EmployerCompany,
  EmployerCompanyProfileInput,
  EmployerTeam,
  EmployerTeamInvitation,
  EmployerTeamInvitationPreview,
  EmployerVacancyInput,
  EmployerVacancyPatch,
  Job,
} from '../types';
import type { Slice } from '../api-types';
import type { components } from '#lib/generated/api.js';
import { ApiError, jsonBody, toSlice } from '../api-core';

export function employerApi({ call, request, requestData, baseUrl }: Http) {
  // ── Verified employer accounts ────────────────────────────────────────────

  /** Claim a company: resolves the name to a slug, refuses a public-webmail work email,
   *  reserves the slug, and mails a verification code. Returns the pending account. */
  async function claimCompany(companyName: string, workEmail: string): Promise<EmployerAccount> {
    return requestData<EmployerAccount>(
      '/api/v1/employer/claim',
      jsonBody('POST', { company_name: companyName, work_email: workEmail }),
    );
  }

  /** Confirm the mailed code. Activates immediately on a matching company domain;
   *  otherwise the account stays pending, visible to moderators. Either way the response
   *  is the (possibly still pending) account. */
  async function confirmEmployerClaim(code: string): Promise<EmployerAccount> {
    return requestData<EmployerAccount>('/api/v1/employer/claim/confirm', jsonBody('POST', { code }));
  }

  /** Mails a fresh work-email code for the caller's still-unconfirmed claim. 429 inside the
   *  resend cooldown; 409 once the email is already confirmed. */
  async function resendEmployerClaimCode(): Promise<EmployerAccount> {
    return requestData<EmployerAccount>('/api/v1/employer/claim/resend', { method: 'POST' });
  }

  /** The caller's own employer account, whatever status it is in, plus the company's
   *  current curated profile once active. Never requires an active account — this is the
   *  one read a pending/revoked claimant can still make. `null` when the caller has never
   *  claimed a company (a 404, not a refusal). */
  async function getEmployerCompany(): Promise<EmployerCompany | null> {
    try {
      return await requestData<EmployerCompany>('/api/v1/employer/company');
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) return null;
      throw err;
    }
  }

  /** Apply the caller's authoritative edit to their own company's curated profile.
   *  Active-account-only. */
  async function updateEmployerCompany(input: EmployerCompanyProfileInput): Promise<EmployerCompany> {
    return requestData<EmployerCompany>(
      '/api/v1/employer/company',
      jsonBody('PATCH', input satisfies components['schemas']['EmployerCompanyPatch']),
    );
  }

  /** Every vacancy the caller has published through this path, newest first, open and
   *  closed both. Active-account-only. */
  async function listEmployerVacancies(): Promise<Job[]> {
    return requestData<Job[]>('/api/v1/employer/jobs');
  }

  /** Publish a new vacancy for the caller's own claimed company, or idempotently update/
   *  reopen one the caller already owns at the same URL. Active-account-only; a URL
   *  already owned by a different employer is a 409. */
  async function createEmployerVacancy(input: EmployerVacancyInput): Promise<Job> {
    return requestData<Job>(
      '/api/v1/employer/jobs',
      jsonBody('POST', input satisfies components['schemas']['EmployerVacancyCreate']),
    );
  }

  /** Edit a vacancy the caller's own account owns, addressed by public slug. Any other
   *  slug (missing, another owner's) is a 404. */
  async function updateEmployerVacancy(slug: string, input: EmployerVacancyPatch): Promise<Job> {
    return requestData<Job>(
      `/api/v1/employer/jobs/${encodeURIComponent(slug)}`,
      jsonBody('PATCH', input satisfies components['schemas']['EmployerVacancyPatch']),
    );
  }

  /** Soft-close a vacancy the caller's own account owns. */
  async function closeEmployerVacancy(slug: string): Promise<void> {
    await call(`/api/v1/employer/jobs/${encodeURIComponent(slug)}/close`, { method: 'POST' });
  }

  /** Undo the caller's own close of their vacancy. A close someone else made is a 400. */
  async function reopenEmployerVacancy(slug: string): Promise<void> {
    await call(`/api/v1/employer/jobs/${encodeURIComponent(slug)}/reopen`, { method: 'POST' });
  }

  // ── The applicant inbox ───────────────────────────────────────────────────

  /** One page of the applications sent on freehire to the caller's vacancies, newest
   *  first, narrowed to one vacancy and/or stage, as a Slice for the shared Paginator.
   *  `stageCounts` counts every stage under the vacancy filter. Active-account-only. */
  async function listEmployerApplications(filter: {
    job?: string;
    stage?: EmployerApplicationStage;
    limit: number;
    offset: number;
  }): Promise<Slice<EmployerApplication> & { stageCounts: Record<string, number> }> {
    const q = new URLSearchParams({ limit: String(filter.limit), offset: String(filter.offset) });
    if (filter.job) q.set('job', filter.job);
    if (filter.stage) q.set('stage', filter.stage);
    const page = await request<EmployerApplicationsPage>(`/api/v1/employer/applications?${q}`);
    return { ...toSlice(page, filter.offset), stageCounts: page.meta.stage_counts };
  }

  /** Where the CV an application sent is served, as it was emailed — a link to open, not a
   *  fetch: the browser renders the PDF itself. */
  function employerApplicationCVUrl(id: number): string {
    return `${baseUrl}/api/v1/employer/applications/${id}/cv`;
  }

  /** Move an application to a stage of the caller's pipeline. `message` goes to the
   *  candidate and only with interview, offer or rejected. */
  async function moveEmployerApplication(
    id: number,
    stage: EmployerApplicationStage,
    message?: string,
  ): Promise<EmployerApplicationMoved> {
    return requestData<EmployerApplicationMoved>(
      `/api/v1/employer/applications/${id}/stage`,
      jsonBody('POST', { stage, message } satisfies components['schemas']['EmployerApplicationMove']),
    );
  }

  // ── The company team ──────────────────────────────────────────────────────

  /** Everyone who acts for the caller's company, and the invitations still open. */
  async function getEmployerTeam(): Promise<EmployerTeam> {
    return requestData<EmployerTeam>('/api/v1/employer/team');
  }

  /** Mail a colleague a single-use link to join the team. Inviting an address again
   *  replaces its link. */
  async function inviteEmployerTeamMember(email: string): Promise<EmployerTeamInvitation> {
    return requestData<EmployerTeamInvitation>(
      '/api/v1/employer/team/invitations',
      jsonBody('POST', { email } satisfies components['schemas']['EmployerTeamInvite']),
    );
  }

  /** Withdraw an invitation; its link stops working. */
  async function withdrawEmployerTeamInvitation(id: number): Promise<void> {
    await call(`/api/v1/employer/team/invitations/${id}`, { method: 'DELETE' });
  }

  /** Remove a member — or the caller, to leave. Their vacancies stay with the team. */
  async function removeEmployerTeamMember(userId: number): Promise<void> {
    await call(`/api/v1/employer/team/members/${userId}`, { method: 'DELETE' });
  }

  /** Which company an invitation link invites to, at which address. 404 for a dead link. */
  async function previewEmployerTeamInvitation(token: string): Promise<EmployerTeamInvitationPreview> {
    return requestData<EmployerTeamInvitationPreview>(
      '/api/v1/employer/team/invitation-preview',
      jsonBody('POST', { token } satisfies components['schemas']['EmployerTeamToken']),
    );
  }

  /** Join the team an invitation link invites to. 409 when the caller already acts for a
   *  company. */
  async function joinEmployerTeam(token: string): Promise<EmployerAccount> {
    return requestData<EmployerAccount>(
      '/api/v1/employer/team/join',
      jsonBody('POST', { token } satisfies components['schemas']['EmployerTeamToken']),
    );
  }

  /** The moderator review queue: employer claims that could not auto-activate. */
  async function listPendingEmployerClaims(): Promise<EmployerPendingClaim[]> {
    return requestData<EmployerPendingClaim[]>('/api/v1/employer/claims');
  }

  /** Activate a pending claim and, when the company's website was still blank, seed it
   *  from the claim's confirmed work-email domain. Moderator-only. */
  async function approveEmployerClaim(userId: number): Promise<EmployerAccount> {
    return requestData<EmployerAccount>(`/api/v1/employer/claims/${userId}/approve`, { method: 'POST' });
  }

  /** Remove a pending claim, freeing its company slug for a future claim. Moderator-only. */
  async function rejectEmployerClaim(userId: number): Promise<void> {
    await call(`/api/v1/employer/claims/${userId}/reject`, { method: 'POST' });
  }

  return {
    claimCompany,
    confirmEmployerClaim,
    resendEmployerClaimCode,
    getEmployerCompany,
    updateEmployerCompany,
    listEmployerVacancies,
    createEmployerVacancy,
    updateEmployerVacancy,
    closeEmployerVacancy,
    reopenEmployerVacancy,
    listEmployerApplications,
    employerApplicationCVUrl,
    moveEmployerApplication,
    getEmployerTeam,
    inviteEmployerTeamMember,
    withdrawEmployerTeamInvitation,
    removeEmployerTeamMember,
    previewEmployerTeamInvitation,
    joinEmployerTeam,
    listPendingEmployerClaims,
    approveEmployerClaim,
    rejectEmployerClaim,
  };
}
