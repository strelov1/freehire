import "@typespec/http";

using Http;

namespace Freehire;

// Employee referrals (session cookie or API key). Not here: the offer upload (multipart)
// and the two proof/CV PDF streams stay hand-written. An id that is not a UUID answers 404,
// like a missing row, so ids travel as strings.

model ReferralOffer {
  id: string;
  company_slug: string;
  company_name: string;
  linkedin_url: string;
  status: string;
  decided_at: utcDateTime | null;
  created_at: utcDateTime | null;
}

/** A seeker's own request: the target and status, no referrer identity. */
model ReferralSeekerRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: int64 | null;
  cv_kind: string;
  cv_id: string | null;
  status: string;
  created_at: utcDateTime | null;
}

/** What a referrer sees: the seeker's chosen contact and CV, never their user id. */
model ReferralIncomingRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: int64 | null;
  cv_kind: string;
  linkedin_url?: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
  status: string;
  created_at: utcDateTime | null;
}

model ReferralRequestCreate {
  company_slug?: string;

  /** Names the vacancy; an unknown slug is a 404. Wins over `job_id`. */
  job_slug?: string;

  job_id?: int64 | null;
  cv_kind?: string;
  cv_id?: string | null;
  linkedin_url?: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
}

model ReferralCVOption {
  id: string;
  title: string;
}

model ReferralCVOptions {
  tailored: ReferralCVOption[];
  has_resume: boolean;
}

/** `status` is "contacted" or "declined". */
model ReferralResolve {
  status?: string;
}

model ReferralDecision {
  approve?: boolean;
}

@tag("Referrals")
@route("/me/referrals/offers")
@get
op listMyReferralOffers(): Data<ReferralOffer[]> | Problem;

/** 404 when the offer is absent or not the caller's. */
@tag("Referrals")
@route("/me/referrals/offers/{id}")
@delete
op withdrawReferralOffer(@path id: string): NoContentResponse | Problem;

/** 422 for a bad request, 409 for a duplicate or a company with no referral available. */
@tag("Referrals")
@route("/me/referrals/requests")
@post
op createReferralRequest(@body body: ReferralRequestCreate): {
  @statusCode _: 201;
  @body body: Data<ReferralSeekerRequest>;
} | Problem;

/** The CVs tailored for the vacancy (or any at the company), and whether a CV file is stored. */
@tag("Referrals")
@route("/me/referrals/cv-options")
@get
op listReferralCVOptions(@query company_slug?: string, @query job_slug?: string): Data<ReferralCVOptions> | Problem;

@tag("Referrals")
@route("/me/referrals/requests")
@get
op listMyReferralRequests(): Data<ReferralSeekerRequest[]> | Problem;

/** The open requests for every company the caller is an approved referrer of. */
@tag("Referrals")
@route("/me/referrals/incoming")
@get
op listIncomingReferralRequests(): Data<ReferralIncomingRequest[]> | Problem;

@tag("Referrals")
@route("/me/referrals/incoming/{id}/resolve")
@post
op resolveReferralRequest(@path id: string, @body body: ReferralResolve): Data<ReferralIncomingRequest> | Problem;

/** Moderators only: the offers awaiting a decision. */
@tag("Referrals")
@route("/referrals/offers")
@get
op listPendingReferralOffers(): Data<ReferralOffer[]> | Problem;

/** Moderators only. */
@tag("Referrals")
@route("/referrals/offers/{id}/decide")
@post
op decideReferralOffer(@path id: string, @body body: ReferralDecision): Data<ReferralOffer> | Problem;
