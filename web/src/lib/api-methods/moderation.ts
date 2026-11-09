import type { Http } from '../api-core';
import type { IncomingReferralRequest, ReferralOffer, Submission } from '../types';
import { jsonBody } from '../api-core';

export function moderationApi({ requestData, baseUrl }: Http) {
  /** The referrer inbox: open requests for the companies the caller is approved for. */
  async function listIncomingReferrals(): Promise<IncomingReferralRequest[]> {
    return requestData<IncomingReferralRequest[]>('/api/v1/me/referrals/incoming');
  }

  /** Mark an incoming request contacted or declined on the caller's behalf. */
  async function resolveReferral(id: string, status: 'contacted' | 'declined'): Promise<IncomingReferralRequest> {
    return requestData<IncomingReferralRequest>(
      `/api/v1/me/referrals/incoming/${encodeURIComponent(id)}/resolve`,
      jsonBody('POST', { status }),
    );
  }

  /** The URL that streams an incoming request's attached CV (opened in a new tab). */
  function referralCvUrl(id: string): string {
    return `${baseUrl}/api/v1/me/referrals/incoming/${encodeURIComponent(id)}/cv`;
  }

  /** The moderator queue: referral offers awaiting a decision, oldest first. */
  async function listPendingReferralOffers(): Promise<ReferralOffer[]> {
    return requestData<ReferralOffer[]>('/api/v1/referrals/offers');
  }

  /** Approve or reject a pending offer. Moderator-only. */
  async function decideReferralOffer(id: string, approve: boolean): Promise<ReferralOffer> {
    return requestData<ReferralOffer>(
      `/api/v1/referrals/offers/${encodeURIComponent(id)}/decide`,
      jsonBody('POST', { approve }),
    );
  }

  /** The URL that streams an offer's proof CV (moderator-only, opened in a new tab). */
  function referralProofUrl(id: string): string {
    return `${baseUrl}/api/v1/referrals/offers/${encodeURIComponent(id)}/proof`;
  }

  /** The moderator review queue: pending submissions, with submitter emails. */
  async function listPendingSubmissions(): Promise<Submission[]> {
    return requestData<Submission[]>('/api/v1/submissions');
  }

  /** Approve a pending submission; the server mints a live job from it. */
  async function approveSubmission(id: number): Promise<Submission> {
    return requestData<Submission>(`/api/v1/submissions/${id}/approve`, {
      method: 'POST',
    });
  }

  /** Reject a pending submission with an optional reason. When blockDomain is set, the
   *  submission's URL host is also added to the submission-domain blocklist and every
   *  other pending submission on that host is rejected in the same action — the response
   *  is still only the target submission, so a caller passing blockDomain should re-fetch
   *  the queue to see any bulk-rejected siblings. */
  async function rejectSubmission(id: number, reason?: string, blockDomain?: boolean): Promise<Submission> {
    return requestData<Submission>(
      `/api/v1/submissions/${id}/reject`,
      jsonBody('POST', { reason: reason ?? '', block_domain: blockDomain ?? false }),
    );
  }

  return {
    listIncomingReferrals,
    resolveReferral,
    referralCvUrl,
    listPendingReferralOffers,
    decideReferralOffer,
    referralProofUrl,
    listPendingSubmissions,
    approveSubmission,
    rejectSubmission,
  };
}
