import "@typespec/http";
import "@typespec/openapi";

using Http;
using TypeSpec.OpenAPI;

namespace Freehire;

// Employee referrals (session cookie or API key). Not here: the offer upload (multipart)
// and the two proof/CV PDF streams stay hand-written. An id that is not a UUID answers 404,
// like a missing row, so ids travel as strings.

enum ReferralOfferStatus {
  pending,
  approved,
  rejected,
}

enum ReferralRequestStatus {
  sent,
  contacted,
  declined,
}

/** `original` attaches the uploaded CV; `built` one tailored in freehire. */
enum ReferralCvKind {
  original,
  built,
}

model ReferralOffer {
  id: string;
  company_slug: string;
  company_name: string;
  linkedin_url: string;
  status: ReferralOfferStatus;
  decided_at: utcDateTime | null;
  created_at: utcDateTime | null;
}

/** A seeker's own request: the target and status, no referrer identity. */
model ReferralSeekerRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: int64 | null;
  cv_kind: ReferralCvKind;
  cv_id: string | null;
  status: ReferralRequestStatus;
  created_at: utcDateTime | null;
}

/** What a referrer sees: the seeker's chosen contact and CV, never their user id. */
model ReferralIncomingRequest {
  id: string;
  company_slug: string;
  company_name: string;
  job_id: int64 | null;
  cv_kind: ReferralCvKind;
  linkedin_url?: string;
  contact_telegram?: string;
  contact_email?: string;
  note?: string;
  status: ReferralRequestStatus;
  created_at: utcDateTime | null;
}

model ReferralRequestCreate {
  /** The company to ask. */
  company_slug?: string;

  /** Names the vacancy; an unknown slug is a 404. Wins over `job_id`. */
  job_slug?: string;

  job_id?: int64 | null;
  /** Which CV to attach: `original` (your uploaded one) or `built`. */
  cv_kind?: string;
  /**
   * The built CV, when `cv_kind` is `built`: one you tailored for that opening, or — with
   * no opening — for any opening at the company (see `cv-options`). Any other CV answers
   * 422.
   */
  cv_id?: string | null;
  /** Your LinkedIn profile URL. */
  linkedin_url?: string;
  /** How the referrer reaches you. */
  contact_telegram?: string;
  /** How the referrer reaches you. */
  contact_email?: string;
  /** A short message to the referrer. */
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
  /** `contacted` or `declined`. (e.g. `contacted`) */
  status?: string;
}

model ReferralDecision {
  /** True approves the referrer; false rejects the offer. */
  approve?: boolean;
}

@summary("Your referral offers and their moderation state.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/offers")
@get
op listMyReferralOffers(): Data<ReferralOffer[]> | Problem;

/** 404 when the offer is absent or not the caller's. */
@summary("Withdraw an offer.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/offers/{id}")
@delete
op withdrawReferralOffer(/** The offer id. */ @path id: string): NoContentResponse | Problem;

// 422 for a bad request, 409 for a duplicate or a company with no referral available.
/**
 * Validation failures answer 422; a company with no approved referrer answers 409; too
 * many open requests answers 429.
 */
@summary("Ask a company's approved referrers for an intro.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/requests")
@post
op createReferralRequest(@body body: ReferralRequestCreate): {
  @statusCode _: 201;
  @body body: Data<ReferralSeekerRequest>;
} | Problem;

// The CVs tailored for the vacancy (or any at the company), and whether a CV file is
// stored.
/**
 * The CVs you tailored for `job_slug`, or with no `job_slug` for any opening at the
 * company, newest first, and whether you have uploaded a CV file.
 */
@summary("The CVs you can attach to a request into a company.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      tailored: #[
        #{
          id: "6f1c…",
          title: "Tailored — Backend Engineer",
        },
      ],
      has_resume: true,
    },
  },
})
@tag("Referrals")
@route("/me/referrals/cv-options")
@get
op listReferralCVOptions(/** The company to ask. */ @query company_slug?: string, /** The opening the request is made from. */ @query job_slug?: string): Data<ReferralCVOptions> | Problem;

@summary("The intros you asked for, and where they stand.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/requests")
@get
op listMyReferralRequests(): Data<ReferralSeekerRequest[]> | Problem;

/** The open requests for every company the caller is an approved referrer of. */
@summary("Requests waiting on you as an approved referrer.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/incoming")
@get
op listIncomingReferralRequests(): Data<ReferralIncomingRequest[]> | Problem;

@summary("Mark an incoming request contacted or declined.")
@useAuth(SessionOrKey)
@tag("Referrals")
@route("/me/referrals/incoming/{id}/resolve")
@post
op resolveReferralRequest(/** The request id. */ @path id: string, @body body: ReferralResolve): Data<ReferralIncomingRequest> | Problem;

/** Moderators only: the offers awaiting a decision. */
@summary("The offer-moderation queue.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Referrals")
@route("/referrals/offers")
@get
op listPendingReferralOffers(): Data<ReferralOffer[]> | Problem;

/** Moderators only. */
@summary("Approve or reject an offer.")
@useAuth(SessionOrKey)
@extension("x-audience", "moderator")
@tag("Referrals")
@route("/referrals/offers/{id}/decide")
@post
op decideReferralOffer(/** The offer id. */ @path id: string, @body body: ReferralDecision): Data<ReferralOffer> | Problem;
