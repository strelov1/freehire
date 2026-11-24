import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// Pay-per-click sponsored jobs: a buyer promotes one open job, pays in through Stripe
// Checkout, a moderator approves it, and clicks on the sponsored link are charged. Money is
// integer USD cents. Session cookie only; every answer is `Cache-Control: no-store`.

/** `draft`, `pending_payment`, `pending_review`, `active`, `paused`, `exhausted`, `rejected` or `ended`. */
model SponsoredCampaign {
  id: int64;
  status: string;
  job_slug: string;
  job_title: string;
  job_company: string;
  cpc_cents: int64;

  /** Absent when there is no daily cap. */
  daily_cap_cents?: int64;

  /** Paid in so far. */
  budget_cents: int64;
  spent_cents: int64;
  remaining_cents: int64;

  /** Set only on a rejected campaign. */
  reject_reason?: string;

  created_at: utcDateTime;
}

model SponsoredCampaignRequest {
  /** The job's public slug (the one in its page URL). */
  job_slug?: string;

  cpc_cents?: int64;

  /** Optional; at least `cpc_cents` when set. */
  daily_cap_cents?: int64;
}

model SponsoredDayStat {
  day: plainDate;
  impressions: int64;
  valid_clicks: int64;
  invalid_clicks: int64;
  spent_cents: int64;
}

model SponsoredInvalidReason {
  /** `bot`, `duplicate`, `rate`, `inactive` or `budget`. */
  reason: string;

  clicks: int64;
}

model SponsoredCampaignDetail {
  campaign: SponsoredCampaign;

  /** The last 30 days that have any activity, oldest first. */
  days: SponsoredDayStat[];

  /** Clicks that were not charged in the last 30 days, grouped by why. */
  invalid_clicks_by_reason: SponsoredInvalidReason[];
}

model SponsoredTopupRequest {
  amount_cents?: int64;
}

model SponsoredCampaignHeaders {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredCampaign>;
}

model SponsoredCampaignCreatedHeaders {
  @statusCode _: 201;
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredCampaign>;
}

model SponsoredCampaignListHeaders {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredCampaign[]>;
}

model SponsoredCampaignDetailHeaders {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredCampaignDetail>;
}

model SponsoredTopupHeaders {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<BillingUrl>;
}

/**
 * Opens a draft campaign for an open, public job. 422 for a cost per click under the
 * minimum (`SPONSORED_MIN_CPC_CENTS`, 10 by default), a daily cap under the click cost, or a
 * job that is closed, private or unknown.
 */
@summary("Create a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/campaigns")
@post
op createSponsoredCampaign(@body body: SponsoredCampaignRequest): SponsoredCampaignCreatedHeaders | Problem;

/** The caller's campaigns, newest first. */
@summary("List your sponsored campaigns.")
@tag("Sponsored")
@route("/sponsored/campaigns")
@get
op listSponsoredCampaigns(): SponsoredCampaignListHeaders | Problem;

/** One of the caller's campaigns with its delivery and spend. Another user's id answers 404. */
@summary("Read a sponsored campaign and its stats.")
@tag("Sponsored")
@route("/sponsored/campaigns/{id}")
@get
op getSponsoredCampaign(@path id: string): SponsoredCampaignDetailHeaders | Problem;

/** `active` to `paused`. 409 when the campaign is in another state. */
@summary("Pause a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/campaigns/{id}/pause")
@post
op pauseSponsoredCampaign(@path id: string): SponsoredCampaignHeaders | Problem;

/** `paused` to `active`. 409 when the campaign is in another state. */
@summary("Resume a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/campaigns/{id}/resume")
@post
op resumeSponsoredCampaign(@path id: string): SponsoredCampaignHeaders | Problem;

/** Ends the campaign for good; unused budget stays recorded. 409 when it is already over. */
@summary("End a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/campaigns/{id}/end")
@post
op endSponsoredCampaign(@path id: string): SponsoredCampaignHeaders | Problem;

/**
 * Returns a Stripe Checkout URL that pays `amount_cents` into the campaign. 422 under the
 * minimum top-up, 409 for a rejected or ended campaign, 404 when payments are not configured.
 */
@summary("Start a sponsored campaign top-up.")
@tag("Sponsored")
@route("/sponsored/campaigns/{id}/topup")
@post
op topupSponsoredCampaign(@path id: string, @body body: SponsoredTopupRequest): SponsoredTopupHeaders | Problem;

model SponsoredReviewItem {
  id: int64;
  job_slug: string;
  job_title: string;
  job_company: string;
  job_url: string;
  cpc_cents: int64;
  daily_cap_cents?: int64;
  budget_cents: int64;

  /** The buyer's account id; no email is shown. */
  owner_user_id: int64;

  created_at: utcDateTime;
}

model SponsoredRejectRequest {
  /** Required and non-empty; shown to the buyer. */
  reason?: string;
}

model SponsoredRejection {
  rejected: boolean;

  /** True when the campaign is rejected but the refund did not finish; it is retried later. */
  refund_pending: boolean;
}

model SponsoredApproval {
  approved: boolean;
}

/** Moderators only, oldest first. */
@extension("x-audience", "moderator")
@summary("List sponsored campaigns waiting for review.")
@tag("Sponsored")
@route("/sponsored/review")
@get
op listSponsoredReview(): {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredReviewItem[]>;
} | Problem;

/** Moderators only. 409 when the campaign is not in review. */
@extension("x-audience", "moderator")
@summary("Approve a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/review/{id}/approve")
@post
op approveSponsoredCampaign(@path id: string): {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredApproval>;
} | Problem;

/** Moderators only. Refunds what was paid; answers 200 with `refund_pending` when the refund is still to retry. */
@extension("x-audience", "moderator")
@summary("Reject a sponsored campaign.")
@tag("Sponsored")
@route("/sponsored/review/{id}/reject")
@post
op rejectSponsoredCampaign(@path id: string, @body body: SponsoredRejectRequest): {
  @header("Cache-Control") cacheControl: string;
  @body response: Data<SponsoredRejection>;
} | Problem;
