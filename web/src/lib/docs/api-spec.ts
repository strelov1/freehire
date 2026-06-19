// The freehire public API, described as data. This single module is the source
// of truth for BOTH the rendered /docs/api page and the generated docs/API.md
// (web/scripts/gen-api-docs.mjs), so the two can never drift. The job-search
// filter vocabulary is NOT duplicated here — it lives in ./filters, derived from
// the generated contracts so it stays in lock-step with the Go StringFacets.

/** Production base URL for every path below. */
export const BASE_URL = 'https://freehire.dev/api/v1';

/** Authentication requirement for an endpoint, rendered as a badge. */
export type Auth = 'none' | 'cookie-or-key' | 'cookie' | 'moderator';

/** Human-readable label for an auth level. */
export const AUTH_LABELS: Record<Auth, string> = {
  none: 'Public',
  'cookie-or-key': 'Session or API key',
  cookie: 'Session only',
  moderator: 'Moderator',
};

/** A single request parameter (path, query, or body field). */
export interface Param {
  name: string;
  type: string;
  required?: boolean;
  description: string;
  example?: string;
}

/** One HTTP endpoint. `curl` and `responseExample` are plain strings so they
 *  drop verbatim into both the page's code blocks and the Markdown fences. */
export interface Endpoint {
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  path: string;
  auth: Auth;
  summary: string;
  description?: string;
  /** Marks the endpoint that carries the full job-search filter vocabulary, so
   *  the renderer can inject the shared filter table after its own params. */
  filterable?: boolean;
  pathParams?: Param[];
  query?: Param[];
  body?: Param[];
  curl: string;
  responseExample?: string;
}

/** A group of related endpoints, rendered as one page section. The anchor is
 *  derived from the title (slugified) by both renderers, like Overview. */
export interface Group {
  title: string;
  intro: string;
  endpoints: Endpoint[];
}

/** A conceptual section before the endpoint reference (base URL, envelope,
 *  pagination, errors, auth model). Kept as paragraphs + an optional code
 *  block so neither renderer needs a Markdown parser. The anchor is derived
 *  from the title (slugified) by both renderers, so there is no separate id. */
export interface Overview {
  title: string;
  paragraphs: string[];
  code?: string;
}

export const OVERVIEW: Overview[] = [
  {
    title: 'Base URL',
    paragraphs: [
      `All endpoints are served under \`${BASE_URL}\`. The API is read-first and ` +
        'open: the job, search, facet, and company endpoints need no authentication ' +
        'and may be called cross-origin.',
      'Authenticated endpoints accept either the browser session cookie (set by ' +
        'sign-in, same-origin) or a personal API key sent as a Bearer token — see ' +
        'Authentication and API keys below.',
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
      'Search pagination is bounded: `offset + limit` may not exceed 10000 ' +
        '(`pagination too deep` → 400). This is deep-paging protection, not a cap ' +
        'on the reported total — use filters to narrow rather than paging that far.',
    ],
  },
  {
    title: 'Errors',
    paragraphs: [
      'Errors use standard HTTP status codes: 400 (bad request / invalid value), ' +
        '401 (missing or invalid credentials), 403 (authenticated but not allowed, ' +
        'e.g. a non-moderator), 404 (no such job, company, or owned resource), and ' +
        '503 (search temporarily unavailable). The body is always `{ "error": ... }`.',
    ],
  },
  {
    title: 'Authentication model',
    paragraphs: [
      'Browser clients authenticate with an `HttpOnly` session cookie set on ' +
        'sign-in (same-origin; the SPA cannot read it). Non-browser clients use a ' +
        'personal API key as `Authorization: Bearer <token>`.',
      'Endpoints marked “Session or API key” accept either; endpoints marked ' +
        '“Session only” (API-key management, saved searches, subscriptions) accept ' +
        'only the cookie, so a leaked key cannot manage credentials. “Moderator” ' +
        'endpoints additionally require the moderator role.',
    ],
  },
];

export const GROUPS: Group[] = [
  {
    title: 'Jobs',
    intro:
      'Public, unauthenticated reads. Jobs are returned in one wire shape ' +
      '(addressed by `public_slug`, never an internal id) shared by the list, ' +
      'detail, company, and search responses. Closed postings are excluded from ' +
      'lists and search and served only by the detail endpoint.',
    endpoints: [
      {
        method: 'GET',
        path: '/jobs',
        auth: 'none',
        summary: 'List jobs, newest first, with limit/offset pagination.',
        query: [
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/jobs?limit=20&offset=0"`,
        responseExample: `{
  "data": [
    {
      "public_slug": "senior-go-engineer-acme-1a2b",
      "title": "Senior Go Engineer",
      "company": "Acme",
      "company_slug": "acme",
      "url": "https://boards.greenhouse.io/acme/jobs/123",
      "location": "Remote — EU",
      "regions": ["europe"],
      "countries": ["DE"],
      "work_mode": "remote",
      "skills": ["go", "postgresql"],
      "collections": ["yc"],
      "source": "greenhouse",
      "posted_at": "2026-06-18T00:00:00Z",
      "enrichment": { "seniority": "senior", "category": "backend" }
    }
  ],
  "meta": { "total": 4213, "limit": 20, "offset": 0 }
}`,
      },
      {
        method: 'GET',
        path: '/jobs/search',
        auth: 'none',
        summary: 'Full-text + faceted search over open jobs.',
        description:
          'Combine free-text `q` with any of the filter params below. Repeated ' +
          'facet params are ORed; add `<param>_mode=and` to require all, or ' +
          '`<param>_exclude=<value>` to exclude. Without `q`, results default to ' +
          'newest first; with `q`, to relevance.',
        filterable: true,
        query: [
          { name: 'q', type: 'string', description: 'Full-text query over title, company, and description.', example: 'golang' },
          { name: 'sort', type: 'string', description: 'One of `created_at`, `posted_at`, `salary_min`, `salary_max`. Omit for relevance/newest.', example: 'posted_at' },
          { name: 'order', type: 'string', description: '`asc` or `desc` (default `desc`).', example: 'desc' },
          { name: 'semantic_ratio', type: 'number', description: 'Opt-in hybrid search, 0–1 (default 0 = pure keyword). Needs the optional semantic index.', example: '0' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip; `offset + limit` ≤ 10000.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/jobs/search?q=golang&seniority=senior&work_mode=remote&regions=cis&sort=posted_at"`,
        responseExample: `{
  "data": [ { "public_slug": "...", "title": "Senior Go Engineer", "...": "..." } ],
  "meta": { "total": 137, "limit": 20, "offset": 0 }
}`,
      },
      {
        method: 'GET',
        path: '/jobs/facets',
        auth: 'none',
        summary: 'Count of matching jobs per facet value (and numeric stats).',
        description:
          'Takes the same `q` and filter params as search, but returns the ' +
          'distribution of values instead of a page of jobs — use it to build ' +
          'filter UIs or see how a filter narrows the set. Continuous numeric ' +
          'facets are returned as `stats` (min/max), not per-value buckets.',
        query: [
          { name: 'q', type: 'string', description: 'Same full-text query as search.', example: 'golang' },
          { name: '(any filter)', type: 'string', description: 'Any search filter param narrows the counted set.', example: 'work_mode=remote' },
        ],
        curl: `curl "${BASE_URL}/jobs/facets?work_mode=remote"`,
        responseExample: `{
  "data": {
    "total": 1820,
    "facets": {
      "seniority": { "senior": 640, "middle": 410, "junior": 120 },
      "category": { "backend": 700, "frontend": 380 }
    },
    "stats": {
      "salary_min": { "min": 20000, "max": 400000 }
    }
  }
}`,
      },
      {
        method: 'GET',
        path: '/jobs/{slug}',
        auth: 'none',
        summary: 'A single job by its public slug (serves closed jobs too).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.', example: 'senior-go-engineer-acme-1a2b' }],
        curl: `curl "${BASE_URL}/jobs/senior-go-engineer-acme-1a2b"`,
        responseExample: `{ "data": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "closed_at": null, "...": "..." } }`,
      },
      {
        method: 'GET',
        path: '/jobs/{slug}/similar',
        auth: 'none',
        summary: 'Jobs similar to the given one (semantic; may be empty).',
        description:
          'Backed by the optional semantic index. Returns an empty list (not an ' +
          'error) when the source job is not indexed.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.', example: 'senior-go-engineer-acme-1a2b' }],
        query: [{ name: 'limit', type: 'integer', description: 'Max similar jobs.', example: '10' }],
        curl: `curl "${BASE_URL}/jobs/senior-go-engineer-acme-1a2b/similar?limit=10"`,
        responseExample: `{ "data": [ { "public_slug": "...", "title": "...", "...": "..." } ] }`,
      },
    ],
  },
  {
    title: 'Companies',
    intro: 'Public reads. A company detail also returns a page of its open jobs.',
    endpoints: [
      {
        method: 'GET',
        path: '/companies',
        auth: 'none',
        summary: 'List companies with job counts; optional name filter.',
        query: [
          { name: 'q', type: 'string', description: 'Case-insensitive name substring filter.', example: 'acme' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/companies?q=acme"`,
        responseExample: `{ "data": [ { "name": "Acme", "slug": "acme", "job_count": 12 } ], "meta": { "total": 1, "limit": 20, "offset": 0 } }`,
      },
      {
        method: 'GET',
        path: '/companies/{slug}',
        auth: 'none',
        summary: 'A company and a page of its open jobs.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
        query: [
          { name: 'limit', type: 'integer', description: 'Page size for the jobs list.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip in the jobs list.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/companies/acme"`,
        responseExample: `{ "data": { "company": { "name": "Acme", "slug": "acme" }, "jobs": [ { "public_slug": "...", "...": "..." } ] } }`,
      },
    ],
  },
  {
    title: 'Authentication',
    intro:
      'Register/login set the session cookie and return the user. Logout clears ' +
      'it. `me` resolves the caller (cookie or API key). OAuth sign-in is a ' +
      'redirect flow. Credential endpoints are rate-limited.',
    endpoints: [
      {
        method: 'POST',
        path: '/auth/register',
        auth: 'none',
        summary: 'Create an account and start a session.',
        body: [
          { name: 'email', type: 'string', required: true, description: 'Account email (canonical key).', example: 'me@example.com' },
          { name: 'password', type: 'string', required: true, description: 'Account password.' },
        ],
        curl: `curl -X POST "${BASE_URL}/auth/register" \\
  -H 'Content-Type: application/json' \\
  -c cookies.txt \\
  -d '{"email":"me@example.com","password":"hunter2hunter2"}'`,
        responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user", "created_at": "2026-06-19T10:00:00Z" } }`,
      },
      {
        method: 'POST',
        path: '/auth/login',
        auth: 'none',
        summary: 'Sign in and start a session.',
        body: [
          { name: 'email', type: 'string', required: true, description: 'Account email.', example: 'me@example.com' },
          { name: 'password', type: 'string', required: true, description: 'Account password.' },
        ],
        curl: `curl -X POST "${BASE_URL}/auth/login" \\
  -H 'Content-Type: application/json' \\
  -c cookies.txt \\
  -d '{"email":"me@example.com","password":"hunter2hunter2"}'`,
        responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user" } }`,
      },
      {
        method: 'POST',
        path: '/auth/logout',
        auth: 'none',
        summary: 'Clear the session cookie.',
        curl: `curl -X POST "${BASE_URL}/auth/logout" -b cookies.txt`,
        responseExample: `{ "data": { "ok": true } }`,
      },
      {
        method: 'GET',
        path: '/auth/me',
        auth: 'cookie-or-key',
        summary: 'The current user (cookie or API key).',
        curl: `curl "${BASE_URL}/auth/me" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "id": 1, "email": "me@example.com", "role": "user" } }`,
      },
      {
        method: 'GET',
        path: '/auth/oauth/providers',
        auth: 'none',
        summary: 'List the enabled OAuth providers.',
        curl: `curl "${BASE_URL}/auth/oauth/providers"`,
        responseExample: `{ "data": ["google", "github"] }`,
      },
      {
        method: 'GET',
        path: '/auth/oauth/{provider}/start',
        auth: 'none',
        summary: 'Begin the OAuth sign-in redirect.',
        description:
          'Browser-only: redirects to the provider, then back to ' +
          '`/auth/oauth/{provider}/callback`, which sets the session cookie and ' +
          'redirects to the app. Not a JSON endpoint.',
        pathParams: [{ name: 'provider', type: 'string', required: true, description: 'One of the enabled providers.', example: 'google' }],
        curl: `# open in a browser:
${BASE_URL}/auth/oauth/google/start`,
      },
    ],
  },
  {
    title: 'API keys',
    intro:
      'Personal keys for non-browser access. Management is session-only (a leaked ' +
      'key cannot mint more keys). The plaintext token is shown exactly once, at ' +
      'creation — store it then.',
    endpoints: [
      {
        method: 'POST',
        path: '/me/api-keys',
        auth: 'cookie',
        summary: 'Create a key; returns the plaintext token once.',
        body: [
          { name: 'name', type: 'string', required: true, description: 'Label to tell keys apart.', example: 'cli-laptop' },
          { name: 'expires_at', type: 'string (RFC3339)', description: 'Optional expiry; omit for no expiry.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/api-keys" \\
  -H 'Content-Type: application/json' \\
  -b cookies.txt \\
  -d '{"name":"cli-laptop"}'`,
        responseExample: `{ "data": { "id": 7, "name": "cli-laptop", "token_prefix": "fh_ab12", "token": "fh_ab12...REDACTED...full-token-shown-once" } }`,
      },
      {
        method: 'GET',
        path: '/me/api-keys',
        auth: 'cookie',
        summary: 'List your keys (metadata only, never the token).',
        curl: `curl "${BASE_URL}/me/api-keys" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": 7, "name": "cli-laptop", "token_prefix": "fh_ab12", "last_used_at": null, "expires_at": null } ] }`,
      },
      {
        method: 'DELETE',
        path: '/me/api-keys/{id}',
        auth: 'cookie',
        summary: 'Revoke a key.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The key id.', example: '7' }],
        curl: `curl -X DELETE "${BASE_URL}/me/api-keys/7" -b cookies.txt`,
        responseExample: `{ "data": { "ok": true } }`,
      },
    ],
  },
  {
    title: 'Job interactions',
    intro:
      'Per-user tracking, addressed by the job slug. All accept the session ' +
      'cookie or an API key and are idempotent. The response is the interaction ' +
      'record for that job.',
    endpoints: [
      {
        method: 'POST',
        path: '/jobs/{slug}/view',
        auth: 'cookie-or-key',
        summary: 'Record that you viewed the job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/view" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "viewed_at": "2026-06-19T10:00:00Z" } }`,
      },
      {
        method: 'POST',
        path: '/jobs/{slug}/apply',
        auth: 'cookie-or-key',
        summary: 'Mark the job as applied to.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/apply" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "applied_at": "2026-06-19T10:00:00Z" } }`,
      },
      {
        method: 'POST',
        path: '/jobs/{slug}/save',
        auth: 'cookie-or-key',
        summary: 'Save (bookmark) the job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/save" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "saved_at": "2026-06-19T10:00:00Z" } }`,
      },
      {
        method: 'DELETE',
        path: '/jobs/{slug}/save',
        auth: 'cookie-or-key',
        summary: 'Unsave the job (no-op if not saved).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/save" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "saved_at": null } }`,
      },
      {
        method: 'PATCH',
        path: '/jobs/{slug}/track',
        auth: 'cookie-or-key',
        summary: 'Set the application stage and/or notes.',
        description:
          'A null field is left unchanged. `stage` is a controlled vocabulary: ' +
          '`applied`, `screening`, `responded`, `interview`, `offer`, `accepted`, ' +
          '`rejected`, `withdrawn` (an unknown value is a 400).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        body: [
          { name: 'stage', type: 'string', description: 'Application stage from the vocabulary above.', example: 'interview' },
          { name: 'notes', type: 'string', description: 'Free-text notes.' },
        ],
        curl: `curl -X PATCH "${BASE_URL}/jobs/<slug>/track" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"stage":"interview","notes":"call on Friday"}'`,
        responseExample: `{ "data": { "job_id": 42, "stage": "interview", "notes": "call on Friday" } }`,
      },
      {
        method: 'DELETE',
        path: '/jobs/{slug}/stage',
        auth: 'cookie-or-key',
        summary: 'Clear the application stage.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/stage" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "stage": null } }`,
      },
      {
        method: 'DELETE',
        path: '/jobs/{slug}/track',
        auth: 'cookie-or-key',
        summary: 'Remove the interaction record entirely.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/track" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "ok": true } }`,
      },
      {
        method: 'GET',
        path: '/me/jobs',
        auth: 'cookie-or-key',
        summary: 'Your tracked jobs joined with the job data.',
        description:
          'Each item carries the job in the shared wire shape with your interaction ' +
          'timestamps alongside it. `meta.counts` gives the per-filter totals for tab ' +
          'badges. Closed jobs stay listed so your history never shrinks.',
        query: [
          { name: 'filter', type: 'string', description: 'Subset to return: `all`, `viewed`, `saved`, `applied`, or `board` (default `all`; an unknown value is a 400).', example: 'applied' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/me/jobs?filter=applied" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": [
    {
      "job": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "...": "..." },
      "viewed_at": "2026-06-19T10:00:00Z",
      "saved_at": null,
      "applied_at": "2026-06-19T11:00:00Z",
      "stage": "interview",
      "notes": "call on Friday"
    }
  ],
  "meta": {
    "total": 5,
    "limit": 20,
    "offset": 0,
    "counts": { "all": 12, "viewed": 12, "saved": 3, "applied": 5, "board": 7 }
  }
}`,
      },
      {
        method: 'GET',
        path: '/me/jobs/viewed',
        auth: 'cookie-or-key',
        summary: 'Slugs of jobs you have viewed.',
        curl: `curl "${BASE_URL}/me/jobs/viewed" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": ["senior-go-engineer-acme-1a2b", "..."] }`,
      },
    ],
  },
  {
    title: 'Job submissions',
    intro:
      'Any signed-in user can submit a vacancy for moderation and read their own ' +
      'queue. The review actions are moderator-only; approval mints a live job.',
    endpoints: [
      {
        method: 'POST',
        path: '/submissions',
        auth: 'cookie-or-key',
        summary: 'Submit a vacancy for review.',
        body: [
          { name: 'url', type: 'string', required: true, description: 'Link to the original posting.', example: 'https://acme.com/careers/123' },
          { name: 'title', type: 'string', required: true, description: 'Job title.', example: 'Senior Go Engineer' },
          { name: 'company', type: 'string', required: true, description: 'Company name.', example: 'Acme' },
          { name: 'location', type: 'string', description: 'Free-text location.', example: 'Remote — EU' },
          { name: 'remote', type: 'boolean', description: 'Whether the role is remote.', example: 'true' },
          { name: 'description', type: 'string', description: 'Job description.' },
          { name: 'source', type: 'string', description: 'Origin hint (optional).' },
          { name: 'posted_at', type: 'string (RFC3339)', description: 'Original posting date (optional).' },
        ],
        curl: `curl -X POST "${BASE_URL}/submissions" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"url":"https://acme.com/careers/123","title":"Senior Go Engineer","company":"Acme","remote":true}'`,
        responseExample: `{ "data": { "id": 9, "status": "pending", "title": "Senior Go Engineer", "company": "Acme", "url": "https://acme.com/careers/123" } }`,
      },
      {
        method: 'GET',
        path: '/me/submissions',
        auth: 'cookie-or-key',
        summary: 'Your own submission queue.',
        curl: `curl "${BASE_URL}/me/submissions" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": [ { "id": 9, "status": "pending", "title": "Senior Go Engineer" } ] }`,
      },
      {
        method: 'GET',
        path: '/submissions',
        auth: 'moderator',
        summary: 'The pending submission queue (moderators).',
        curl: `curl "${BASE_URL}/submissions" -H "Authorization: Bearer $MODERATOR_API_KEY"`,
        responseExample: `{ "data": [ { "id": 9, "status": "pending", "submitter_email": "me@example.com" } ] }`,
      },
      {
        method: 'POST',
        path: '/submissions/{id}/approve',
        auth: 'moderator',
        summary: 'Approve a submission, minting a live job.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The submission id.', example: '9' }],
        curl: `curl -X POST "${BASE_URL}/submissions/9/approve" -H "Authorization: Bearer $MODERATOR_API_KEY"`,
        responseExample: `{ "data": { "id": 9, "status": "approved", "job_slug": "senior-go-engineer-acme-1a2b" } }`,
      },
      {
        method: 'POST',
        path: '/submissions/{id}/reject',
        auth: 'moderator',
        summary: 'Reject a submission with a reason.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The submission id.', example: '9' }],
        body: [{ name: 'reason', type: 'string', description: 'Why it was rejected.', example: 'duplicate' }],
        curl: `curl -X POST "${BASE_URL}/submissions/9/reject" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"reason":"duplicate"}'`,
        responseExample: `{ "data": { "id": 9, "status": "rejected", "review_reason": "duplicate" } }`,
      },
    ],
  },
  {
    title: 'Job reports',
    intro:
      'Any signed-in user can flag a problem with a live vacancy. Review actions ' +
      'are moderator-only; resolving may soft-close the reported job.',
    endpoints: [
      {
        method: 'POST',
        path: '/jobs/{slug}/reports',
        auth: 'cookie-or-key',
        summary: 'Report a problem with a job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        body: [
          { name: 'reason', type: 'string', required: true, description: 'Short reason code/category.', example: 'expired' },
          { name: 'details', type: 'string', description: 'Free-text details.' },
          { name: 'contact_telegram', type: 'string', description: 'Optional contact handle.' },
        ],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/reports" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"reason":"expired","details":"posting returns 404"}'`,
        responseExample: `{ "data": { "id": 3, "status": "pending", "reason": "expired" } }`,
      },
      {
        method: 'GET',
        path: '/reports',
        auth: 'moderator',
        summary: 'The pending report queue (moderators).',
        curl: `curl "${BASE_URL}/reports" -H "Authorization: Bearer $MODERATOR_API_KEY"`,
        responseExample: `{ "data": [ { "id": 3, "status": "pending", "job_slug": "...", "job_title": "..." } ] }`,
      },
      {
        method: 'POST',
        path: '/reports/{id}/resolve',
        auth: 'moderator',
        summary: 'Resolve a report, optionally closing the job.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
        body: [{ name: 'close_job', type: 'boolean', description: 'Soft-close the reported job.', example: 'true' }],
        curl: `curl -X POST "${BASE_URL}/reports/3/resolve" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"close_job":true}'`,
        responseExample: `{ "data": { "id": 3, "status": "resolved" } }`,
      },
      {
        method: 'POST',
        path: '/reports/{id}/dismiss',
        auth: 'moderator',
        summary: 'Dismiss a report with a reason.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
        body: [{ name: 'reason', type: 'string', description: 'Why it was dismissed.', example: 'not an issue' }],
        curl: `curl -X POST "${BASE_URL}/reports/3/dismiss" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"reason":"not an issue"}'`,
        responseExample: `{ "data": { "id": 3, "status": "dismissed", "review_reason": "not an issue" } }`,
      },
    ],
  },
  {
    title: 'Moderator jobs',
    intro:
      'Hand-curate a vacancy directly (moderators only). Approved submissions go ' +
      'through the same minting path, so a curated job is indistinguishable from ' +
      'an approved one.',
    endpoints: [
      {
        method: 'POST',
        path: '/jobs',
        auth: 'moderator',
        summary: 'Create a curated job.',
        body: [
          { name: 'url', type: 'string', required: true, description: 'Link to the posting.', example: 'https://acme.com/careers/123' },
          { name: 'source', type: 'string', description: 'Source label.', example: 'manual' },
          { name: 'title', type: 'string', required: true, description: 'Job title.', example: 'Senior Go Engineer' },
          { name: 'company', type: 'string', required: true, description: 'Company name.', example: 'Acme' },
          { name: 'location', type: 'string', description: 'Free-text location.' },
          { name: 'remote', type: 'boolean', description: 'Whether the role is remote.' },
          { name: 'description', type: 'string', description: 'Job description.' },
          { name: 'posted_at', type: 'string (RFC3339)', description: 'Posting date.' },
        ],
        curl: `curl -X POST "${BASE_URL}/jobs" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"url":"https://acme.com/careers/123","title":"Senior Go Engineer","company":"Acme"}'`,
        responseExample: `{ "data": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "manually_added": true } }`,
      },
      {
        method: 'PATCH',
        path: '/jobs/{slug}',
        auth: 'moderator',
        summary: 'Edit a curated job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        body: [{ name: '(any job field)', type: 'varies', description: 'Same fields as create; provided fields are updated.' }],
        curl: `curl -X PATCH "${BASE_URL}/jobs/<slug>" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"title":"Staff Go Engineer"}'`,
        responseExample: `{ "data": { "public_slug": "...", "title": "Staff Go Engineer" } }`,
      },
    ],
  },
  {
    title: 'Saved searches & subscriptions',
    intro:
      'Browser conveniences, session-only. A saved search stores a canonical ' +
      'filter query string; a subscription turns one into a recurring digest ' +
      '(e.g. Telegram). Each operation is owner-scoped — a non-owned id is a 404.',
    endpoints: [
      {
        method: 'GET',
        path: '/me/searches',
        auth: 'cookie',
        summary: 'List your saved searches.',
        curl: `curl "${BASE_URL}/me/searches" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": 2, "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote" } ] }`,
      },
      {
        method: 'POST',
        path: '/me/searches',
        auth: 'cookie',
        summary: 'Save a search.',
        body: [
          { name: 'name', type: 'string', required: true, description: 'Display name.', example: 'Senior Go remote' },
          { name: 'query', type: 'string', required: true, description: 'Canonical filter query string.', example: 'q=go&seniority=senior&work_mode=remote' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/searches" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"name":"Senior Go remote","query":"q=go&seniority=senior&work_mode=remote"}'`,
        responseExample: `{ "data": { "id": 2, "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote" } }`,
      },
      {
        method: 'PATCH',
        path: '/me/searches/{id}',
        auth: 'cookie',
        summary: 'Rename or re-query a saved search.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
        body: [
          { name: 'name', type: 'string', description: 'New name (optional).' },
          { name: 'query', type: 'string', description: 'New query (optional).' },
        ],
        curl: `curl -X PATCH "${BASE_URL}/me/searches/2" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"name":"Senior Go — EU remote"}'`,
        responseExample: `{ "data": { "id": 2, "name": "Senior Go — EU remote", "query": "..." } }`,
      },
      {
        method: 'DELETE',
        path: '/me/searches/{id}',
        auth: 'cookie',
        summary: 'Delete a saved search.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
        curl: `curl -X DELETE "${BASE_URL}/me/searches/2" -b cookies.txt`,
        responseExample: `{ "data": { "ok": true } }`,
      },
      {
        method: 'GET',
        path: '/me/subscriptions',
        auth: 'cookie',
        summary: 'List your subscriptions.',
        curl: `curl "${BASE_URL}/me/subscriptions" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": 1, "saved_search_id": 2, "channel": "telegram", "active": true } ] }`,
      },
      {
        method: 'POST',
        path: '/me/subscriptions',
        auth: 'cookie',
        summary: 'Subscribe a saved search to a digest channel.',
        body: [
          { name: 'saved_search_id', type: 'integer', required: true, description: 'The saved search to subscribe.', example: '2' },
          { name: 'channel', type: 'string', required: true, description: 'Delivery channel.', example: 'telegram' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/subscriptions" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"saved_search_id":2,"channel":"telegram"}'`,
        responseExample: `{ "data": { "id": 1, "saved_search_id": 2, "channel": "telegram", "active": true } }`,
      },
      {
        method: 'PATCH',
        path: '/me/subscriptions/{id}',
        auth: 'cookie',
        summary: 'Pause or resume a subscription.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The subscription id.', example: '1' }],
        body: [{ name: 'active', type: 'boolean', required: true, description: 'Whether the subscription is active.', example: 'false' }],
        curl: `curl -X PATCH "${BASE_URL}/me/subscriptions/1" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"active":false}'`,
        responseExample: `{ "data": { "id": 1, "active": false } }`,
      },
      {
        method: 'DELETE',
        path: '/me/subscriptions/{id}',
        auth: 'cookie',
        summary: 'Delete a subscription.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The subscription id.', example: '1' }],
        curl: `curl -X DELETE "${BASE_URL}/me/subscriptions/1" -b cookies.txt`,
        responseExample: `{ "data": { "ok": true } }`,
      },
      {
        method: 'GET',
        path: '/me/telegram',
        auth: 'cookie',
        summary: 'Your Telegram link status (for digests).',
        curl: `curl "${BASE_URL}/me/telegram" -b cookies.txt`,
        responseExample: `{ "data": { "enabled": true, "linked": true, "chat_id": 123456789 } }`,
      },
      {
        method: 'POST',
        path: '/me/telegram/link',
        auth: 'cookie',
        summary: 'Start linking your Telegram account.',
        curl: `curl -X POST "${BASE_URL}/me/telegram/link" -b cookies.txt`,
        responseExample: `{ "data": { "url": "https://t.me/free_hire_bot?start=..." } }`,
      },
      {
        method: 'DELETE',
        path: '/me/telegram',
        auth: 'cookie',
        summary: 'Unlink your Telegram account.',
        curl: `curl -X DELETE "${BASE_URL}/me/telegram" -b cookies.txt`,
        responseExample: `{ "data": { "ok": true } }`,
      },
    ],
  },
];
