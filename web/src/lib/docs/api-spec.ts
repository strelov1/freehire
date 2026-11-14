// The freehire public API, described as data. This single module is the source
// of truth for BOTH the rendered /docs/api page and the generated docs/API.md
// (web/scripts/gen-api-docs.mjs), so the two can never drift. The job-search
// filter vocabulary is NOT duplicated here — it lives in ./filters, derived from
// the generated contracts so it stays in lock-step with the Go StringFacets.

import { BASE_URL, type Group, type Overview } from './api-spec-types';
import { jobsGroup } from './api-spec-groups/jobs';
import { aiAnalysisGroup } from './api-spec-groups/ai-analysis';
import { companiesGroup } from './api-spec-groups/companies';
import { geographyGroup } from './api-spec-groups/geography';
import { companyFeedbackGroup } from './api-spec-groups/company-feedback';
import { authenticationGroup } from './api-spec-groups/authentication';
import { jobInteractionsGroup } from './api-spec-groups/job-interactions';
import { inAppAssistantGroup } from './api-spec-groups/in-app-assistant';
import { jobSubmissionsGroup } from './api-spec-groups/job-submissions';
import { jobReportsGroup } from './api-spec-groups/job-reports';
import { ghostJobReportsGroup } from './api-spec-groups/ghost-job-reports';
import { moderatorJobsGroup } from './api-spec-groups/moderator-jobs';
import { profileResumeGroup } from './api-spec-groups/profile-resume';
import { screeningAnswersGroup } from './api-spec-groups/screening-answers';
import { onboardingSurveyGroup } from './api-spec-groups/onboarding-survey';
import { activityGroup } from './api-spec-groups/activity';
import { savedSearchesSubscriptionsGroup } from './api-spec-groups/saved-searches-subscriptions';
import { pushNotificationsAlertsGroup } from './api-spec-groups/push-notifications-alerts';
import { accountPlanExtensionGroup } from './api-spec-groups/account-plan-extension';
import { linkIntakeDiscoveryGroup } from './api-spec-groups/link-intake-discovery';
import { votesNotificationsDiscussionsGroup } from './api-spec-groups/votes-notifications-discussions';
import { marketInsightsStatsGroup } from './api-spec-groups/market-insights-stats';
import { marketPulseGroup } from './api-spec-groups/market-pulse';
import { employeeReferralsGroup } from './api-spec-groups/employee-referrals';
import { talentNetworkGroup } from './api-spec-groups/talent-network';
import { cvBuilderTailoringGroup } from './api-spec-groups/cv-builder-tailoring';
import { photoGroup } from './api-spec-groups/photo';
import { experienceBankGroup } from './api-spec-groups/experience-bank';
import { applicationMailGroup } from './api-spec-groups/application-mail';
import { speechGroup } from './api-spec-groups/speech';

export * from './api-spec-types';

export const OVERVIEW: Overview[] = [
  {
    title: 'Base URL',
    paragraphs: [
      `All endpoints are served under \`${BASE_URL}\`. The API is read-first and ` +
        'open: the job, search, facet, and company endpoints need no authentication ' +
        'and may be called cross-origin.',
      'Authenticated endpoints accept either the browser session cookie (set by ' +
        'sign-in, same-origin) or a personal API key sent as a Bearer token — see the ' +
        'authentication model below. Keys are created in your account, at ' +
        '[freehire.me/my/integrations/api-keys](https://freehire.me/my/integrations/api-keys), not through this API.',
    ],
  },
  {
    title: 'Response envelope',
    paragraphs: [
      'Single items are wrapped as `{ "data": ... }`. Collections add pagination ' +
        'metadata: `{ "data": [...], "meta": { "total", "limit", "offset" } }`. ' +
        'Errors are `{ "error": "message" }` with a matching HTTP status.',
    ],
    code: `{ "data": { "...": "single item" } }

{ "data": [ ... ], "meta": { "total": 4213, "limit": 20, "offset": 0 } }

{ "error": "job not found" }`,
  },
  {
    title: 'Pagination',
    paragraphs: [
      'List and search endpoints page with `limit` (default 20, max 100) and ' +
        '`offset` (default 0). `meta.total` reports the total matching the current ' +
        'filters, so you can compute the number of pages.',
      'Pagination is bounded on **every** list endpoint: `offset + limit` may not ' +
        'exceed 10000 (`pagination too deep` → 400). This is deep-paging protection, ' +
        'not a cap on the reported total — use filters to narrow rather than paging ' +
        'that far.',
      'The bound is a refusal rather than a clamp, deliberately: a clamped page would ' +
        'answer 200 carrying rows you did not ask for, and a client walking pages would ' +
        'loop over the same page forever without being able to tell. A 400 means ' +
        '"there is no such page", which is the truth.',
    ],
  },
  {
    title: 'Errors',
    paragraphs: [
      'Errors use standard HTTP status codes: 400 (bad request / invalid value), ' +
        '401 (missing or invalid credentials), 403 (authenticated but not allowed, ' +
        'e.g. a non-moderator), 404 (no such job, company, or owned resource), ' +
        '429 (request limit exceeded), and 503 (service temporarily unavailable). ' +
        'Application errors use `{ "error": ... }`. nginx rate-limit rejections ' +
        'can return a non-JSON body; see API plans below.',
    ],
  },
  {
    title: 'API plans',
    paragraphs: [
      'The public read endpoints answer without a key for personal and non-commercial use. ' +
        'Showing the postings in a product you offer to others needs an ' +
        '[API plan](https://freehire.me/jobs-api), under the ' +
        '[data licence](https://freehire.me/terms#data-licence).',
      'Send a full-access key as `Authorization: Bearer <token>`, or use the OAuth access ' +
        'token of a signed-in MCP or CLI connection.',
      'The API plan allows 1,200 requests per 60-second window. The Data plan allows ' +
        '6,000. Neither paid API plan has a daily request cap. The account shares one ' +
        'budget across its full-access API keys and OAuth connections, including public ' +
        'REST reads and agent search. A browser session cookie does not select the paid API rate.',
      'The limit counts HTTP requests, not returned listings or unique vacancies. ' +
        'A page containing 100 listings costs one request. Re-fetching the same page or ' +
        'vacancy costs another request, including when the data has not changed. ' +
        'There is no separate cooldown for refreshing a vacancy.',
      'Pagination returns 20 listings by default and up to 100 with `limit=100`. ' +
        'Requests carrying `offset` or `company_slug` also pass through separate ' +
        'per-IP nginx limits of 2 requests per second each, including paid requests. ' +
        'The `offset` limit allows a burst of 50 excess requests; the `company_slug` ' +
        'limit allows 30. A request carrying both parameters draws on both limits. ' +
        'For sustained pagination or company-filtered reads, keep the combined traffic ' +
        'from one IP within the applicable 2-request-per-second limit.',
      'The gateway returns `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and ' +
        '`X-RateLimit-Reset` (seconds until reset). A window starts with its first ' +
        'request. Exceeding a request limit returns HTTP 429. Gateway rejections ' +
        'include `Retry-After` in seconds; nginx rejections can omit these headers ' +
        'and return a non-JSON body. Pause for `Retry-After` when present; otherwise ' +
        'back off before retrying.',
    ],
  },
  {
    title: 'Authentication model',
    paragraphs: [
      'Browser clients authenticate with an `HttpOnly` session cookie set on ' +
        'sign-in (same-origin; the SPA cannot read it). Non-browser clients use a ' +
        'personal API key as `Authorization: Bearer <token>`.',
      'Endpoints marked “Session or API key” accept either; endpoints marked ' +
        '“Session only” (saved searches, subscriptions) accept ' +
        'only the cookie, so a leaked key cannot manage credentials. “Moderator” ' +
        'endpoints additionally require the moderator role.',
    ],
  },
  {
    title: 'What is not here',
    paragraphs: [
      'This reference covers every endpoint you can call. A handful are ' +
        'deliberately left out because calling them directly is meaningless: the ' +
        'Gmail and calendar consent redirects (`/me/gmail/connect`, ' +
        '`/me/calendar/connect`, and their callbacks), which only a browser can ' +
        'complete; the Telegram bot webhook; the ' +
        'browser-tool websocket relay; the sitemap-cursor helpers behind ' +
        '`/sitemap.xml`; and the `/og/*.png` social-preview cards, which render an ' +
        'image rather than answer with JSON.',
      // This entry replaces what used to be a full "API keys" endpoint group, curl
      // examples and all. Those recipes could not work: key management is cookie-only AND
      // gated on a short-lived proof of recent credential control, so a scripted call
      // answers 428 no matter what the reader does. Readers took the documented command at
      // face value and got stuck, which is a worse outcome than not documenting it.
      'Managing API keys (`POST`, `GET` and `DELETE` on `/me/api-keys`) is not ' +
        'documented here, because no API client can call it. Those endpoints accept only ' +
        'the browser session cookie, and additionally require a freshly proven identity — ' +
        'a password re-entry or a sign-in-provider check — that a script has no way to ' +
        'supply. Create and revoke keys in your account instead, at ' +
        '[freehire.me/my/integrations/api-keys](https://freehire.me/my/integrations/api-keys); the plaintext token is ' +
        'shown exactly once, at creation, so store it then.',
      'The `/jobs/{slug}/fit` endpoints are pre-rename aliases of ' +
        '`/jobs/{slug}/match-analysis` and hit the same handlers. They still work, ' +
        'so existing clients do not break — use the match-analysis paths in new code.',
      'Most of this API is also a CLI. If you are writing an agent rather than an ' +
        'integration, `freehire` covers the same surface with less ceremony — ' +
        'search, tracking, the inbox and CV tailoring — over one API key.',
    ],
  },
];

export const GROUPS: Group[] = [
  jobsGroup,
  aiAnalysisGroup,
  companiesGroup,
  geographyGroup,
  companyFeedbackGroup,
  authenticationGroup,
  jobInteractionsGroup,
  inAppAssistantGroup,
  jobSubmissionsGroup,
  jobReportsGroup,
  ghostJobReportsGroup,
  moderatorJobsGroup,
  profileResumeGroup,
  screeningAnswersGroup,
  onboardingSurveyGroup,
  activityGroup,
  savedSearchesSubscriptionsGroup,
  pushNotificationsAlertsGroup,
  accountPlanExtensionGroup,
  linkIntakeDiscoveryGroup,
  votesNotificationsDiscussionsGroup,
  marketInsightsStatsGroup,
  marketPulseGroup,
  employeeReferralsGroup,
  talentNetworkGroup,
  cvBuilderTailoringGroup,
  photoGroup,
  experienceBankGroup,
  applicationMailGroup,
  speechGroup,
];
