import type { Http } from '../api-core';
import type { ReferralCvOptions, ReferralOffer, ReferralRequestInput, SeekerReferralRequest } from '../types';
import type { components } from '#lib/generated/api.js';
import { jsonBody } from '../api-core';

export function referralsApi({ call, requestData }: Http) {
  // ── Employee referrals ────────────────────────────────────────────────────

  /** Request a referral into a company's approved-referrer pool. 409 when the company has
   *  no referrer or an active request already exists; 422 on a bad CV/contact; 429 on cap. */
  async function createReferralRequest(input: ReferralRequestInput): Promise<SeekerReferralRequest> {
    return requestData<SeekerReferralRequest>(
      '/api/v1/me/referrals/requests',
      jsonBody('POST', input satisfies components['schemas']['ReferralRequestCreate']),
    );
  }

  /** The CVs the caller can attach to a referral request into a company, about a vacancy
   *  when the request is made from one. */
  async function getReferralCvOptions(companySlug: string, jobSlug?: string): Promise<ReferralCvOptions> {
    const q = new URLSearchParams({ company_slug: companySlug });
    if (jobSlug) q.set('job_slug', jobSlug);
    return requestData<ReferralCvOptions>(`/api/v1/me/referrals/cv-options?${q}`);
  }

  /** The caller's own referral requests, newest first. */
  async function listMyReferralRequests(): Promise<SeekerReferralRequest[]> {
    return requestData<SeekerReferralRequest[]>('/api/v1/me/referrals/requests');
  }

  /** Submit an offer to refer into a company: a proof CV (PDF) uploaded as multipart, with
   *  the company slug and the referrer's LinkedIn URL as form fields. Enters moderation.
   *  409 on a duplicate offer, 422 on a bad LinkedIn URL. */
  async function submitReferralOffer(companySlug: string, linkedinUrl: string, file: File): Promise<ReferralOffer> {
    const form = new FormData();
    form.append('company_slug', companySlug);
    form.append('linkedin_url', linkedinUrl);
    form.append('file', file);
    return requestData<ReferralOffer>('/api/v1/me/referrals/offers', { method: 'POST', body: form });
  }

  /** The caller's own referral offers with moderation status, newest first. */
  async function listMyReferralOffers(): Promise<ReferralOffer[]> {
    return requestData<ReferralOffer[]>('/api/v1/me/referrals/offers');
  }

  /** Stop being a referrer: delete one of the caller's own offers. */
  async function withdrawReferralOffer(id: string): Promise<void> {
    await call(`/api/v1/me/referrals/offers/${encodeURIComponent(id)}`, { method: 'DELETE' });
  }

  return {
    createReferralRequest,
    getReferralCvOptions,
    listMyReferralRequests,
    submitReferralOffer,
    listMyReferralOffers,
    withdrawReferralOffer,
  };
}
