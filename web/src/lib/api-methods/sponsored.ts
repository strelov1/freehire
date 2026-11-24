import type { Http } from '../api-core';
import { jsonBody } from '../api-core';
import type { components } from '#lib/generated/api.js';

type Schemas = components['schemas'];
export type SponsoredCampaign = Schemas['SponsoredCampaign'];
export type SponsoredCampaignDetail = Schemas['SponsoredCampaignDetail'];
export type SponsoredReviewItem = Schemas['SponsoredReviewItem'];

export function sponsoredApi({ requestData }: Http) {
  const base = '/api/v1/sponsored';
  const act = (id: number, verb: 'pause' | 'resume' | 'end') =>
    requestData<SponsoredCampaign>(`${base}/campaigns/${id}/${verb}`, { method: 'POST' });

  return {
    createSponsoredCampaign: (body: Schemas['SponsoredCampaignRequest']) =>
      requestData<SponsoredCampaign>(`${base}/campaigns`, jsonBody('POST', body)),
    listSponsoredCampaigns: () => requestData<SponsoredCampaign[]>(`${base}/campaigns`),
    getSponsoredCampaign: (id: number) => requestData<SponsoredCampaignDetail>(`${base}/campaigns/${id}`),
    pauseSponsoredCampaign: (id: number) => act(id, 'pause'),
    resumeSponsoredCampaign: (id: number) => act(id, 'resume'),
    endSponsoredCampaign: (id: number) => act(id, 'end'),
    /** Starts a Stripe Checkout for the top-up; the caller sends the browser to the returned url. */
    topupSponsoredCampaign: (id: number, amountCents: number) =>
      requestData<Schemas['BillingUrl']>(
        `${base}/campaigns/${id}/topup`,
        jsonBody('POST', { amount_cents: amountCents } satisfies Schemas['SponsoredTopupRequest']),
      ),
    listSponsoredReview: () => requestData<SponsoredReviewItem[]>(`${base}/review`),
    approveSponsoredCampaign: (id: number) =>
      requestData<Schemas['SponsoredApproval']>(`${base}/review/${id}/approve`, { method: 'POST' }),
    rejectSponsoredCampaign: (id: number, reason: string) =>
      requestData<Schemas['SponsoredRejection']>(
        `${base}/review/${id}/reject`,
        jsonBody('POST', { reason } satisfies Schemas['SponsoredRejectRequest']),
      ),
  };
}
