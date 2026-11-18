import "@typespec/http";

using Http;

namespace Freehire;

// The CV-first onboarding's match search. The CV read itself (POST /onboarding/cv) is
// multipart and stays hand-written Fiber, like its siblings /cv/roast and
// /me/resume/extract.

/** What the onboarding searches with: the CV's skills and the answers as a jobs-search query. */
model OnboardingMatchesRequest {
  /** Canonical skill tags, at most 50. They rank the answer; they do not filter it. */
  @maxItems(50)
  skills: string[];

  /** A `/jobs/search` query string (`work_mode=remote&posted_within_days=7`, ...). An unknown param is named in `meta.ignored_params`. */
  query?: string;
}

/** One matching posting with how well its skills match the given ones. */
model OnboardingMatch {
  /** The jobview wire shape `/jobs/search` answers with. */
  job: unknown;

  /** The share of the posting's skills the given skills cover, 0-100. */
  match_percent: int32;
}

/**
 * Open postings ranked by how well their skills match a caller-supplied skill list, under
 * the jobs-search filters in `query`. Public: it is what the CV-first onboarding shows a
 * visitor who has no account and so no stored profile. Answers the top ten.
 */
@tag("Onboarding")
@useAuth(NoAuth)
@opExample(#{
  parameters: #{
    body: #{
      skills: #["go", "kubernetes", "postgresql"],
      query: "work_mode=remote&category=backend&posted_within_days=7",
    },
  },
})
@route("/onboarding/matches")
@post
op onboardingMatches(@body body: OnboardingMatchesRequest): List<OnboardingMatch> | Problem;
