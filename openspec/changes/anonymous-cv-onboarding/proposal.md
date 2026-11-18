## Why

Today a visitor has to create an account before freehire shows them anything personal: the
seven-step `/onboarding` tour only runs after sign-up, and the only anonymous CV path
(`/roast`) scores the CV but carries nothing into an account. A CV-first flow that shows
real, personal results first and asks for an account after (the pattern The Anti Job Board
uses, walked through on 2026-11-12) gives the visitor a reason to sign up. It also replaces
a long tour with one short path.

## What Changes

- **BREAKING (product):** the post-signup seven-step tour (CV, feed, alerts, AI assistant,
  challenge, tailoring, extension) is removed. `/onboarding` becomes the new CV-first flow,
  open to anonymous visitors and to signed-in accounts.
- New flow on `/onboarding`:
  1. Drop a PDF CV. No account needed.
  2. The CV is parsed by the model, shown live: name, headline, location, years,
     companies, key skills.
  3. Three quick questions: where (remote / the CV's country / anywhere), next role
     (from the CV and our catalogue, with open-job counts), and the old tour's "what's in
     your way" survey (multi-select, saved to the survey at sign-up). A visitor who
     already has an account gets a "Sign in" link on the first step.
  4. A short "searching" phase over our own catalogue, with real catalogue figures.
  5. A compact results screen: a logo stack of the matching employers, the top five
     matching open jobs from the last 7 days (**open, not blurred**) one line each with
     their skill-match percent, a link to the feed, and the CV's ATS score as a scorecard.
  6. "Save my matches" goes to the site's sign-in page and back. A new account gets the CV,
     the search profile built from the answers, an email profile alert and the survey
     answers, and lands in its feed.
  7. The old tour's product introduction (tips, AI connector, tailoring, extension) is
     parked on a signed-in page, not linked yet.
- **New free-AI exception:** an anonymous visitor's CV parse runs the model for free. This
  is the fourth `plan.WithFreeAI` mark, beside the stored-CV parse, search interpretation
  and the support agent. It is bounded **per IP** at the operations gateway (the app holds
  no request counters).
- New public endpoints: an anonymous CV parse (stores nothing) and an anonymous match
  search ranked by the posted skills.
- A companion change in the private `freehire-ops` repo adds the per-IP budget to the rate
  gateway's `policy.json`.

## Capabilities

### New Capabilities
- `cv-first-onboarding`: the anonymous CV-first flow. It covers the anonymous model parse
  and its per-IP bound, the questions, the match search and results screen, sign-up from
  the results, and carrying CV, profile and alert into the new account.

### Modified Capabilities
- `onboarding-tour`: every requirement is removed; the tour is replaced by
  `cv-first-onboarding`, which keeps its challenge survey and tips.

## Impact

- **Backend (`internal/api/handler`):** a new anonymous CV-parse route reusing
  `readResumeUpload`, `resumeProfile`, `resumeextract`, `atscheck` and the guest
  service-credential pattern from `search_intent.go`. A new match-search route that ranks
  by a skill vector built from posted skills. The `openRoutes` table in `paid_ai_test.go`
  gets updated. TypeSpec contract for the JSON route.
- **Docs:** root `AGENTS.md` (paid gate: "THREE" marks → four), `internal/ai/plan/AGENTS.md`,
  and the `WithFreeAI` doc comment.
- **Web:**
  - `/onboarding` is rewritten.
  - The old step components and `onboardingSteps`, `onboardingSave` and `onboardingImport`
    are deleted.
  - The layout gate keeps routing a not-yet-onboarded account to `/onboarding` once; the
    apply-gate "Sign up" buttons go to `/signin` and back to the job.
- **Ops:** the `freehire-ops` `policy.json` gets a guest-IP budget for the new parse
  route, published on the active origin's nginx (strelov1/freehire-ops#275).
- **No migration**: nothing new is stored server-side before sign-up.
