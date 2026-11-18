## Context

See proposal.md for the why. The current state that shapes the approach:

**CV parsing**
- A signed-in CV upload (`POST /me/resume/extract`, `PUT /me/resume`) stores the PDF in S3 and
  runs `resumeextract.Extractor.Extract` in a background goroutine under `plan.WithFreeAI`.
  It goes through `llmBinding.caller`, which resolves the user's own gateway credential.
- `resumeextract.Structured` carries name, headline, location, total years, experience,
  skills and more. It has no category or seniority; those come from the dictionaries in
  `resumeProfile` (`skilltag`, `classify`).
- Calling the binding with user id 0 would mint a credential for account 0. The guest
  pattern is `search_intent.go`: `h.llm.client.As("", nil, llm.Feature(tag))`.

**The anonymous roast**
- `POST /cv/roast` is anonymous and calls no model.
- It is deterministic: dictionary profile, `atscheck.Score`, market verdict.
- It stores nothing and carries nothing into an account.

**Rate limits**
- The app holds no request counters. Commit `09e2cd074` moved every HTTP budget to the
  operations gateway.
- That gateway is the OpenResty Lua module in `freehire-ops`
  (`provision/host2/nginx/freehire-rate-gateway/policy.json`).
- Budgets are keyed by identity (`ip`, `guest_ip_only`, `authenticated_user`, …) and bound
  to method+path. `cvroast` and `searchintent-guest` are the precedents.

**Search**
- `GET /jobs/search?sort=match` ranks by cosine against
  `skillvec.ProfileVector(profile.Skills)`, but only for a signed-in caller with a stored
  profile (`matchProfileFor`).
- `search.FilterFromValues` builds every filter we need: `work_mode`, `countries`,
  `salary_min`, `company_type`, `company_size`, `category`, `seniority`,
  `posted_within_days`.

**Web**
- `/onboarding` is the seven-step tour (`components/onboarding/*`, `onboardingSteps`,
  `onboardingSave`, `onboardingImport`).
- The root layout routes accounts with `onboarding_completed_at == null` there. JobView,
  JobRow and JobsView link to it through `onboardingUrl`.
- Sign-up exists only inline in `/signin/+page.svelte`. OAuth is a full-page round trip,
  so in-page state is lost.
- Public pages are English-only, with hardcoded strings.

## Goals / Non-Goals

**Goals:**
- One flow for anonymous and signed-in visitors, from CV drop to results in about a
  minute.
- No server-side storage of anything before an account exists.
- Reuse what exists: the upload reader, the dictionaries, `resumeextract`, `atscheck`, the
  skill-vector ranking, `JobRow`, `JobMatchBar`, the profile-alert helpers and the stored-CV
  upload.

**Non-Goals:**
- Hiring-manager contacts, generated outreach emails and funding-signal feeds. We have no
  such data, and generating emails is paid AI.
- Localising the flow (public pages are English).
- Changing `/roast`, which stays as the standalone ATS checker.
- Telegram or push alert setup inside the flow (the settings pages keep it).

## Decisions

### D1. Two new routes, not an extension of `/cv/roast` or `/jobs/search`

- `POST /api/v1/onboarding/cv` takes a multipart PDF, like `/cv/roast`. It is mounted
  under `mw.optional` and stores nothing.
- `POST /api/v1/onboarding/matches` takes a JSON body and is open to everyone.

Roast's contract is "no model call", and its tests and gateway budget rely on that. Adding
a model call to it would change a published, rate-limited behaviour. A `match_skills`
param on `/jobs/search` would make the public search answer depend on caller-supplied
ranking input for every client (MCP, CLI, saved searches). Dedicated routes keep both
contracts as they are.

The parse route stays hand-written Fiber: it is multipart, like its siblings
`/cv/roast` and `/me/resume/extract`. The matches route goes into the TypeSpec contract
(`typespec/onboarding.tsp`).

### D2. The parse answer

`readResumeUpload` → `resumeProfile` (dictionary skills, categories, seniority) →
`atscheck.Score` (same inputs as roast) → model `Extract`. The answer:

```text
{data: {
  structured_available: bool,
  first_name, headline, location, country (ISO code, resolved via the location dictionary),
  total_years, companies (≤4, most recent first),
  skills, categories, seniority,
  report: atscheck.Report
}}
```

The fields from the model are empty when it is unavailable. Only the professional
projection is used: contact fields are never in the answer (no email, phone or links),
and the full name is cut to the first name.

### D3. The anonymous model call

- **Guest:** `h.llm.client.As("", nil, llm.Feature(tagCVOnboard))`. The context is marked
  `plan.WithFreeAI` so the free exception is explicit and greppable. It is the fourth mark,
  documented with the other three in `internal/ai/plan/AGENTS.md`, the `WithFreeAI` doc
  comment, the root AGENTS.md paid-gate bullet, and the `openRoutes` table in
  `paid_ai_test.go`.
- **Signed-in caller:** `h.llm.caller(plan.WithFreeAI(ctx), userID, …)`, the same as the
  stored-CV parse.

**Timeout.** The extraction runs in the request with a 45 s deadline. That is under
nginx's default 60 s `proxy_read_timeout` on `/api/`, so no ops timeout override is
needed. On timeout or error the answer degrades to the dictionary fields
(`structured_available: false`).

**Alternatives rejected:**
- *Async job + polling.* It needs server-side state for an anonymous caller, which
  conflicts with "store nothing".
- *A longer nginx override.* An extra ops snippet just to wait longer, when a degraded
  answer is acceptable.

### D4. Per-IP bound lives in the gateway

A companion commit to `freehire-ops` adds the budget
`"onboarding-cv-guest": {identity: "guest_ip_only", limit: 5, window_seconds: 86400,
exemptions: ["trusted_peer"]}` and binds it to `POST /api/v1/onboarding/cv` with
`auth_mode: "optional"`. The gateway resolves the session, and `guest_ip_only` counts only
a request with no identity, so signed-in callers do not draw on it. This is the same
binding as `searchintent-guest`.

The app adds no counter: its README states nginx owns every inbound budget. Re-adding an
in-app limiter would reverse that decision.

The web reads a 429 as "daily limit reached" and offers sign-in. We check that the
gateway's validator accepts a 24 h window when the policy is edited. If it does not, the
fallback is the largest accepted window with a proportionally smaller limit, recorded in
the ops commit.

### D5. Match ranking from posted skills

`matchProfileFor` gains a sibling that builds `skillvec.ProfileVector(skills)` from an
explicit list, and the matches handler calls the shared search core (`runJobSearch`) with
that vector and `sort=match`. The handler:

- parses the body's `query` (a jobs-search query string) with `search.FilterFromValues`
  and reports unknown params through `meta.ignored_params`;
- answers with job views plus `match_percent` per job (`skillvec.MatchPercent(job.Skills,
  skills)`) and the total;
- uses limit 10;
- caps `skills` at 50 entries.

The 7-day and 30-day windows are the web's choice, sent as `posted_within_days`. The
endpoint does not decide them.

### D6. Carrying into the account: the browser keeps the CV and the existing endpoints do the saving

**Before sign-up.** The page keeps a draft in IndexedDB (`onboardingDraft.ts`): the PDF
`Blob`, the parse answer, the answers and a timestamp. IndexedDB survives the OAuth round
trip (same origin) and holds a multi-MB blob, which sessionStorage does not. A draft older
than 24 h is ignored and deleted.

**After the account exists.** The page sees `isAuthenticated()` together with a draft and
runs the carry steps independently. Each one is reported:

1. upload the CV through the stored-CV upload (`PUT /me/resume` via `api.putResume`, which
   surfaces a storage failure — unlike `/me/resume/extract`, which swallows it — and starts
   the free stored-CV parse);
2. `profileStore.save` with the profile mapped from the answers;
3. `enableProfileAlert(profile, 'email')`;
4. `api.completeOnboarding()`;
5. delete the draft;
6. go to the feed.

**Alternative rejected: a server-side claim token.** The anonymous parse would store the
CV and fields under a token, and `Register` or the OAuth callback would claim it. That
needs a new table, retention, a sweep and four account-creation paths (v1/v2 OAuth, Apple,
password; `attributeInvite` is already missing from two of them). It would also keep an
anonymous person's CV on our servers. The cost of the browser approach is that a
converting visitor's CV is parsed by the model twice. That happens only for visitors who
sign up, and the second parse is the existing free stored-CV parse.

### D7. Answers → filters and profile (pure, unit-tested `onboardingFlow.ts`)

| Answer | Search filter | Profile field |
|---|---|---|
| where = remote | `work_mode=remote` | remote preference |
| where = country | `countries=<code>` | that country |
| where = anywhere / none | — | — |
| in your way ∋ visa sponsorship | `visa_sponsorship=true` | — (saved to the survey) |
| in your way ∋ remote/flexible, where named no place | `work_mode=remote` | — (saved to the survey) |
| next role | `category=<c>` + `seniority=<s>` | specialization `c`, level `s` |
| (always) | skills sent to the match search | the CV's dictionary skills |

Role choices come from our own catalogue (changed 2026-11-12 after the first look).
The web counts open jobs per `category` through the public `/jobs/facets`:
- the query is the CV's skills (any of them) under the where answer, with no
  role, level or time window;
- the CV's own categories that have jobs come first;
- the largest other categories fill up to four;
- each choice shows its count, and a category with no open jobs is not offered.

Every choice is labelled with `seniority` when the CV gave one. While the counts are
loading, or if the request fails, the choices fall back to the CV's categories alone.

As built, the search query is not mapped separately. It is
`filtersFromProfile(profile to be saved)` minus skills, plus the two challenge filters and the
time window, so the saved profile and its alert reproduce the flow's search by
construction:
- the role answer sends `seniority` together with `seniority_include_unspecified=true`;
- "country" is saved as `remote.countries=[c]` with base `c`;
- "no preference" on the role saves every CV category, with no seniority.

The location mapping uses the existing `LocationPreferences` shape. Its exact field
mapping is settled in implementation against `filtersFromProfile`, so the saved profile
and its alert reproduce the flow's own search.

There is no salary question. It was built and then dropped (2026-11-12): `salary_min`
excludes every posting that states no salary, which is most of the catalogue, and the
question added a step without changing the matches most visitors see.

### D8. Page structure and design

`/onboarding` is a normal page (not the full-screen overlay): a centred `max-w-3xl` column,
footer hidden as today. The steps reveal one under another in a single card:

1. drop zone;
2. "Reading your CV" with a spinner, which becomes a profile summary with `EntityLogo`
   initials for employers and skill `Chip`s;
3. the questions with progress chips;
4. the searching lines with lucide check icons;
5. the results.

Built only from `#lib/ui` primitives (`Card`, `Chip`, `Badge`, `Button` with one `cta` per
view), app components `JobRow` (with `JobMatchBar` fed the endpoint's percent),
`RangeSlider`, and a compact ATS summary (overall score plus three suggestions, reusing
`ATSReportView`'s tone rules). Design tokens only; no new colours.

Sign-up is not embedded. The results screen has one button, "Sign in or create an
account". It marks the draft (`pendingSignupAt`) and goes to `/signin` with `returnTo=/onboarding`.
The page then decides on arrival (`arrivalAction`):
- a marked draft and a new account: carry at once;
- a marked draft and an account that had already been onboarded: the explicit save;
- no marker, or one older than 30 minutes: discard.

An embedded form was built first and dropped (2026-11-12). The site's own sign-in page is
the one sign-in surface, and the redirect costs nothing the draft does not already cover:
OAuth leaves the page anyway.

State is a small explicit phase machine
(`drop → parsing → questions → searching → results → carrying`) held in the page. Its
pure transitions and mappings live in `onboardingFlow.ts`.

### D9. Removal and the gate

**Deleted:**
- the step components (`CvStep`, `FeedStep`, `AlertChannelsStep`, `AgentStep`,
  `ChallengeStep`, `TailorStep`, `ExtensionStep`, `FeatureGrid`, `ProBar`,
  `SproutIllustration`);
- `onboardingSteps.ts`, `onboardingSave.ts`, `onboardingImport.ts` and their tests;
- whatever else they alone used.

**Kept:** `onboardingResumeWait`, `profileAlertSync`, `profileAlert`, `onboarding.ts` (the
feed banner) and `onboardingGate`.

**The gate.** The layout gate stays: accounts with onboarding not complete are routed to
`/onboarding` once and marked complete when shown. The reverse bounce that sends a
completed account away from `/onboarding` is removed, because the page is now useful to
anyone. The anonymous redirect to `/signin` is removed.

**Copy.** `/signin`'s register subtitle ("We'll ask for your CV and a few details next.")
is updated, since a CV may already have been given.

## Risks / Trade-offs

- **[Model cost abuse from rotating IPs]** → a per-IP daily budget at the gateway,
  12 000-rune input truncation in `Extract`, and the 45 s deadline. Spend is attributed
  under its own `feature:` tag, so it is visible.
- **[A CV sent to the model for an anonymous person]** → the same PII redaction
  (`pii.Build`) as the stored parse. Nothing is persisted, and contact fields never leave
  the server.
- **[Double parse on conversion]** → accepted (D6). Only converting visitors pay it.
- **[Company or role filters empty the results]** → widen 7 → 30 days. The results
  screen shows the real count and window. The visitor can go back and change an answer.
- **[OAuth return loses the draft]** (private browsing that blocks IndexedDB) → the page
  falls back to an ordinary signed-in flow and asks for the CV again. The account is
  still created.
- **[Gateway policy not deployed when the app ships]** → the route works unbounded until
  the ops commit lands. The ops policy is deployed first (an unknown route binding is
  harmless before the app has it).

## Migration Plan

1. In `freehire-ops`, add the budget and route binding, then deploy the gateway policy
   to host2 (`provision/host2/AGENTS.md` procedure).
2. Merge the app PR. Autodeploy releases it.
3. Check on prod: an anonymous parse returns structured fields, the sixth parse from one
   IP gets 429, a sign-up carries the CV, profile and alert.
4. Rollback: `rollback.sh` flips the app back. The policy binding can stay; it is inert
   without the route.

No database migration. `onboarding_completed_at` keeps its meaning.

## Later change: the company question gave way to the old tour's survey (2026-11-12)

The company-type question was dropped. It is not a profile field, and its filter thinned
the results. The old tour's "What's in your way?" question took its place as the last
question.

It keeps everything the old challenge step had:
- the vocabulary (`surveyOptions.ts`, restored unchanged), multi-select, and "just
  starting" excluding every other answer;
- the tips table, now under "What helps" on the results;
- saving to the candidate survey, as a fourth independent carry step
  (`updateSurvey({biggest_challenges, biggest_challenge_note})`), run only when something
  was picked.

Two answers also narrow the search, because the catalogue has a filter for them:
- visa sponsorship;
- remote or flexible work, when the where answer named no place.

## Later changes from review on the running page (2026-11-12)

- **Results are compact.** A logo stack (at most ten) plus the top five matches, one line
  each, then a link to the feed with the same query. `JobRow` is not used, and the
  `skillMatch` prop once added to it was reverted.
- **ATS scorecard** (`AtsScorecard.svelte`): a small ring in the band colour, the band word
  (`atsVerdict`), the top three fixes and a link to `/roast`.
- **Removed from the results.** "What helps" and "Keep these matches coming" are gone.
  The introduction is parked on `/my/getting-started`, signed in and not linked: tips
  from the stored survey plus the old tour's connector, tailoring and extension sections.
- **Links open in a new tab.** Job, feed and CV-check links do; the sign-in and save
  actions stay in the tab, because the flow has to come back to it.
- **Remote is bounded by the CV's country.** With a resolved country, "remote" saves
  the remote reach `regions: [global, <region>]`, `countries: [<country>]`.
- **Draft answers are snapshotted before saving.** IndexedDB stores by structured clone,
  which refuses a Svelte `$state` proxy. The draft store swallowed that failure and lost
  the sign-up mark: a real sign-up then discarded the draft. The browser run found it, and
  a spec now structured-clones every saved draft.
