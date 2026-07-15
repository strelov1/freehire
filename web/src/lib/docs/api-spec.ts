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
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
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
      "source": "greenhouse",
      "manually_added": false,
      "external_id": "123",
      "url": "https://boards.greenhouse.io/acme/jobs/123",
      "title": "Senior Go Engineer",
      "company": "Acme",
      "company_slug": "acme",
      "location": "Remote — EU",
      "description": "...",
      "countries": ["DE"],
      "regions": ["eu"],
      "work_mode": "remote",
      "skills": ["go", "postgresql"],
      "cities": ["Berlin"],
      "collections": ["yc"],
      "is_tech": "tech",
      "posted_at": "2026-06-18T00:00:00Z",
      "created_at": "2026-06-18T09:12:00Z",
      "updated_at": "2026-06-18T09:12:00Z",
      "closed_at": null,
      "enrichment": {
        "summary": "...",
        "seniority": "senior",
        "category": "backend",
        "employment_type": "full_time",
        "relocation": "not_supported",
        "visa_sponsorship": false,
        "experience_years_min": 5,
        "english_level": "b2",
        "education_level": "bachelor",
        "domains": ["fintech"],
        "posting_language": "en",
        "company_type": "product",
        "company_size": "51-200",
        "salary_min": 90000,
        "salary_max": 130000,
        "salary_currency": "EUR",
        "salary_period": "year"
      },
      "enriched_at": "2026-06-18T09:20:00Z",
      "enrichment_version": 7,
      "view_count": 42,
      "applied_count": 8,
      "reality": {
        "class": "fresh",
        "age_days": 3,
        "repost_count": 0,
        "mass_posting_count": 0,
        "fake_freshness": false
      }
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
      {
        method: 'GET',
        path: '/jobs/{slug}/copies',
        auth: 'none',
        summary: 'Other open postings in the same role cluster (per-city duplicates).',
        description:
          'The per-city openings folded under one canonical card by content-dedup — ' +
          'each keeps its own `location` and `apply_url` so a seeker picks their city. ' +
          'The anchor job itself is included. `meta.total` is the whole cluster size, ' +
          'so it stays accurate when the list is a capped page.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.', example: 'senior-go-engineer-acme-1a2b' }],
        query: [
          { name: 'limit', type: 'integer', description: 'Page size, 1–200 (default 50).', example: '50' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/jobs/senior-go-engineer-acme-1a2b/copies"`,
        responseExample: `{
  "data": [
    {
      "public_slug": "senior-go-engineer-acme-1a2b",
      "location": "Berlin, Germany",
      "apply_url": "https://boards.greenhouse.io/acme/jobs/123",
      "posted_at": "2026-06-18T00:00:00Z"
    }
  ],
  "meta": { "total": 4 }
}`,
      },
    ],
  },
  {
    title: 'AI analysis',
    intro:
      'Personalized signals computed against the caller’s profile or stored CV. ' +
      'All accept the session cookie or an API key. The skill-match endpoint is ' +
      'deterministic (no LLM); the fit endpoints run the LLM chain and are quota-' +
      'limited. All take the same facet filter params as search where they narrow a ' +
      'market or candidate set.',
    endpoints: [
      {
        method: 'GET',
        path: '/jobs/{slug}/match',
        auth: 'cookie-or-key',
        summary: 'Deterministic skill match of the job against your profile (no LLM).',
        description:
          'How well the job’s skills are covered by your profile skills — exact, ' +
          'adjacent, and missing, plus a coverage percent. A caller without a saved ' +
          'profile is a 404.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl "${BASE_URL}/jobs/<slug>/match" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": {
    "total": 12,
    "exact_count": 7,
    "adjacent_count": 2,
    "coverage_percent": 75,
    "matched": ["go", "postgresql"],
    "adjacent": [ { "name": "typescript", "via": "javascript" } ],
    "missing": ["kubernetes"]
  }
}`,
      },
      {
        method: 'GET',
        path: '/jobs/{slug}/fit',
        auth: 'cookie-or-key',
        summary: 'The cached AI fit analysis for the job (never runs the LLM).',
        description:
          'Returns the cached analysis, flagged `stale` when your CV or the job ' +
          'changed since it was computed, or a null analysis when none is cached. ' +
          '`has_cv` is false when you have no stored CV. `quota` reports your monthly ' +
          'fit-analysis usage.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl "${BASE_URL}/jobs/<slug>/fit" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": {
    "has_cv": true,
    "stale": false,
    "analysis": {
      "overall_score": 82,
      "verdict": "Strong Fit",
      "dimensions": { "...": "..." },
      "requirement_match": [ { "text": "5+ years Go", "priority": "required", "status": "covered", "evidence": "..." } ],
      "strengths": ["..."],
      "gaps": ["..."],
      "recommendation": "..."
    },
    "quota": { "used": 3, "limit": 10, "remaining": 7 }
  }
}`,
      },
      {
        method: 'POST',
        path: '/jobs/{slug}/fit',
        auth: 'cookie-or-key',
        summary: 'Run the three-stage AI fit analysis and cache it.',
        description:
          'Runs the fit prompt-chain over your stored CV and the job, caches the ' +
          'result, and returns it fresh (no `quota` on this response). A new job over ' +
          'your monthly quota is a `429`; recomputing an already-analyzed job is free. ' +
          '`has_cv` is false when no CV is stored; a failing or unconfigured LLM ' +
          'returns a null analysis (200).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/fit" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": {
    "has_cv": true,
    "stale": false,
    "analysis": { "overall_score": 82, "verdict": "Strong Fit", "...": "..." }
  }
}`,
      },
      {
        method: 'GET',
        path: '/jobs/{slug}/fit/stream',
        auth: 'cookie-or-key',
        summary: 'Run the fit analysis over Server-Sent Events.',
        description:
          'The same three-stage chain as `POST /jobs/{slug}/fit`, streamed as SSE ' +
          '(`text/event-stream`) rather than a single JSON body. Each event’s `kind` ' +
          'is one of `stage_start`, `stage_done`, `thinking`, `requirements`, ' +
          '`dimensions`, `final`; the `final` event carries the completed `analysis` ' +
          '(the same shape as the fit endpoints). Not a JSON endpoint.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -N "${BASE_URL}/jobs/<slug>/fit/stream" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `data: {"kind":"stage_start","stage":1,"label":"Extracting requirements"}

data: {"kind":"requirements","requirements":[ { "...": "..." } ]}

data: {"kind":"final","analysis":{"overall_score":82,"verdict":"Strong Fit","...":"..."}}`,
      },
      {
        method: 'GET',
        path: '/me/recommendations',
        auth: 'cookie-or-key',
        summary: 'Open jobs ranked by semantic similarity to your CV.',
        description:
          'Ranks jobs by your persisted CV embedding, constrained by the same facet ' +
          'filter params as search. Degrades to a successful empty list (never an ' +
          'error) when you have no usable CV vector or the semantic index is off.',
        query: [
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip; `offset + limit` ≤ 10000.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/me/recommendations" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": [ { "public_slug": "...", "title": "...", "...": "..." } ],
  "meta": { "total": 40, "limit": 20, "offset": 0 }
}`,
      },
      {
        method: 'POST',
        path: '/market/coverage',
        auth: 'cookie-or-key',
        summary: 'Score a supplied skill list against the filtered job market.',
        description:
          'Stateless sibling of the CV verdict: skills come from the request body, the ' +
          'market from the facet query params (same vocabulary as search; the `skills` ' +
          'facet is ignored as a filter). Reports how many of the role’s vacancies ' +
          'your skills cover and which missing skill unlocks the most. `400` on empty ' +
          'skills, `503` when search is unavailable.',
        query: [
          { name: '(any search filter)', type: 'string', description: 'Any search facet param scopes the market (the `skills` facet is ignored here).', example: 'category=backend' },
        ],
        body: [
          { name: 'skills', type: 'string[]', required: true, description: 'The skill list to score (max 100).', example: '["go","postgresql"]' },
        ],
        curl: `curl -X POST "${BASE_URL}/market/coverage?category=backend" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"skills":["go","postgresql"]}'`,
        responseExample: `{
  "data": {
    "total": 1820,
    "covered": 1400,
    "coverage_percent": 77,
    "gaps": [ { "name": "kubernetes", "new_vacancies": 120, "unlock_percent": 7 } ],
    "skills": [ { "name": "go", "market_frequency": 61, "must_have": true, "status": "strong", "advice": "" } ],
    "must_have_total": 8,
    "must_have_covered": 6,
    "stack_match_percent": 75,
    "bundles": [ { "...": "..." } ]
  }
}`,
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
        summary: 'List companies with job counts and denormalized facets; optional filters.',
        description:
          'Most active first. Facet params are repeatable and filter by array overlap ' +
          '(OR within a facet, AND across facets), composably with `q`. `meta.total` ' +
          'reports the count matching the full filter.',
        query: [
          { name: 'q', type: 'string', description: 'Case-insensitive name substring filter.', example: 'acme' },
          { name: 'collections', type: 'string', description: 'Curated-collection slug (e.g. `yc`, `bigtech`). Repeatable.', example: 'yc' },
          { name: 'regions', type: 'string', description: 'Region the company hires in. Repeatable.', example: 'eu' },
          { name: 'countries', type: 'string', description: 'ISO 3166-1 alpha-2 country. Repeatable.', example: 'DE' },
          { name: 'domains', type: 'string', description: 'Business domain (e.g. `fintech`). Repeatable.', example: 'fintech' },
          { name: 'company_type', type: 'string', description: 'Company type (e.g. `product`, `outstaff`). Repeatable.', example: 'product' },
          { name: 'company_size', type: 'string', description: 'Size bucket (e.g. `51-200`). Repeatable.', example: '51-200' },
          { name: 'remote_regions', type: 'string', description: 'Job-derived remote-hiring region. Repeatable.', example: 'eu' },
          { name: 'yc_batch', type: 'string', description: 'YC batch (e.g. `W21`). Repeatable.', example: 'W21' },
          { name: 'yc_status', type: 'string', description: 'YC company status. Repeatable.', example: 'active' },
          { name: 'yc_stage', type: 'string', description: 'YC funding stage. Repeatable.', example: 'series-a' },
          { name: 'yc_flags', type: 'string', description: 'Curated YC highlight flag. Repeatable.', example: 'hiring' },
          { name: 'maturity', type: 'string', description: 'Company stage/maturity. Repeatable.', example: 'growth' },
          { name: 'subindustries', type: 'string', description: 'YC subindustry leaf (see /companies/subindustries). Repeatable.', example: 'payments' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/companies?q=acme&collections=yc"`,
        responseExample: `{
  "data": [
    {
      "slug": "acme",
      "name": "Acme",
      "job_count": 12,
      "collections": ["yc"],
      "regions": ["eu"],
      "countries": ["DE"],
      "domains": ["fintech"],
      "company_types": ["product"],
      "company_sizes": ["51-200"],
      "industries": ["payments"],
      "year_founded": 2015,
      "employee_count": 120,
      "hq_country": "DE",
      "organization_type": "private",
      "tagline": "Payments for builders",
      "company_info": { "...": "..." },
      "remote_regions": ["eu"],
      "yc_batch": ["W21"],
      "yc_status": ["active"],
      "yc_stage": ["series-a"],
      "yc_flags": ["hiring"],
      "maturity": "growth"
    }
  ],
  "meta": { "total": 1, "limit": 20, "offset": 0 }
}`,
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
        responseExample: `{
  "data": {
    "company": {
      "slug": "acme",
      "name": "Acme",
      "job_count": 12,
      "collections": ["yc"],
      "regions": ["eu"],
      "countries": ["DE"],
      "domains": ["fintech"],
      "company_types": ["product"],
      "company_sizes": ["51-200"],
      "industries": ["payments"],
      "year_founded": 2015,
      "employee_count": 120,
      "hq_country": "DE",
      "organization_type": "private",
      "tagline": "Payments for builders",
      "company_info": { "...": "..." },
      "remote_regions": ["eu"],
      "yc_batch": ["W21"],
      "yc_status": ["active"],
      "yc_stage": ["series-a"],
      "yc_flags": ["hiring"],
      "maturity": "growth"
    },
    "jobs": [ { "public_slug": "...", "title": "...", "...": "..." } ]
  }
}`,
      },
      {
        method: 'GET',
        path: '/companies/subindustries',
        auth: 'none',
        summary: 'Distinct company subindustry vocabulary with company counts.',
        description:
          'Backs the searchable “Industry” facet’s option list, most common first. ' +
          'Counts are unconditional (they do not reflect other active list filters).',
        curl: `curl "${BASE_URL}/companies/subindustries"`,
        responseExample: `{ "data": [ { "value": "payments", "count": 42 }, { "value": "developer-tools", "count": 31 } ] }`,
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
        method: 'POST',
        path: '/jobs/{slug}/dismiss',
        auth: 'cookie-or-key',
        summary: 'Dismiss (swipe away) the job.',
        description:
          'Only keeps the job out of the swipe deck; it stays visible in the public ' +
          '`/jobs` list and search. Idempotent.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/dismiss" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "dismissed_at": "2026-06-19T10:00:00Z", "saved_at": null, "stage": null } }`,
      },
      {
        method: 'DELETE',
        path: '/jobs/{slug}/dismiss',
        auth: 'cookie-or-key',
        summary: 'Undismiss the job (no-op if not dismissed).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X DELETE "${BASE_URL}/jobs/<slug>/dismiss" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": { "job_id": 42, "dismissed_at": null } }`,
      },
      {
        method: 'GET',
        path: '/me/tracking',
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
        curl: `curl "${BASE_URL}/me/tracking?filter=applied" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
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
        path: '/me/tracking/viewed',
        auth: 'cookie-or-key',
        summary: 'Slugs of jobs you have viewed.',
        curl: `curl "${BASE_URL}/me/tracking/viewed" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": ["senior-go-engineer-acme-1a2b", "..."] }`,
      },
      {
        method: 'GET',
        path: '/me/tracking/analyses',
        auth: 'cookie-or-key',
        summary: 'Jobs you have run the AI fit analysis on.',
        description:
          'Newest first, closed jobs included (with `closed: true`). Each item carries the ' +
          'overall score and verdict; `stale` marks an analysis whose CV, job, or model has ' +
          'changed since. `meta.quota` reports your monthly fit-analysis usage. Never runs the LLM.',
        curl: `curl "${BASE_URL}/me/tracking/analyses" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": [
    {
      "slug": "senior-go-engineer-acme-1a2b",
      "title": "Senior Go Engineer",
      "company": "Acme",
      "closed": false,
      "overall_score": 82,
      "verdict": "Strong Fit",
      "analysed_at": "2026-07-11T10:00:00Z",
      "stale": false
    }
  ],
  "meta": { "quota": { "used": 3, "limit": 10, "remaining": 7 } }
}`,
      },
      {
        method: 'GET',
        path: '/me/tracking/saved',
        auth: 'cookie-or-key',
        summary: 'Slugs of jobs you have saved.',
        description: 'Lets the SPA render the save toggle as filled without authenticating the public job reads.',
        curl: `curl "${BASE_URL}/me/tracking/saved" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{ "data": ["senior-go-engineer-acme-1a2b", "..."] }`,
      },
      {
        method: 'GET',
        path: '/me/tracking/pipeline',
        auth: 'cookie-or-key',
        summary: 'Your application-pipeline snapshot (counts per stage bucket).',
        description:
          'The total application count and its distribution across the seven status ' +
          'buckets, aggregated server-side over all of your applications.',
        curl: `curl "${BASE_URL}/me/tracking/pipeline" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": {
    "applications": 12,
    "buckets": {
      "no_answer": 4,
      "in_progress": 3,
      "interviewing": 2,
      "offer": 1,
      "accepted": 1,
      "rejected": 1,
      "declined": 0
    }
  }
}`,
      },
      {
        method: 'GET',
        path: '/me/tracking/swipe',
        auth: 'cookie-or-key',
        summary: 'A batch of open jobs for the swipe triage deck.',
        description:
          'Runs the same query as search (same facets, `q`, and sort), then excludes ' +
          'the jobs you have already saved or dismissed. `503` when search is ' +
          'unavailable.',
        query: [
          { name: 'q', type: 'string', description: 'Optional full-text query (as in search).', example: 'golang' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip; `offset + limit` ≤ 10000.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/me/tracking/swipe" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": [ { "public_slug": "...", "title": "Senior Go Engineer", "...": "..." } ],
  "meta": { "total": 137, "limit": 20, "offset": 0 }
}`,
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
    title: 'Profile & résumé',
    intro:
      'Your career profile and stored CV, session-only (a browser feature, like ' +
      'saved searches). The profile is a singleton keyed by your session — no id in ' +
      'the path. The verdict and ATS report are read-only sub-resources computed from ' +
      'the profile and CV; résumé storage degrades to `501` when object storage is ' +
      'unconfigured.',
    endpoints: [
      {
        method: 'GET',
        path: '/me/profile',
        auth: 'cookie',
        summary: 'Your career profile, or null if you have not saved one.',
        curl: `curl "${BASE_URL}/me/profile" -b cookies.txt`,
        responseExample: `{
  "data": {
    "specializations": ["backend"],
    "skills": ["go", "postgresql"],
    "location_preferences": { "work_modes": ["remote"], "base": { "country": "DE" }, "...": "..." },
    "created_at": "2026-06-19T10:00:00Z",
    "updated_at": "2026-06-19T10:00:00Z"
  }
}`,
      },
      {
        method: 'PUT',
        path: '/me/profile',
        auth: 'cookie',
        summary: 'Create or replace your profile.',
        description:
          'The whole profile is replaced on each save. An unknown specialization ' +
          '(must be a job category), empty skills, or an out-of-vocabulary location ' +
          'value is a `400`.',
        body: [
          { name: 'specializations', type: 'string[]', required: true, description: 'One or more job categories (max 5).', example: '["backend"]' },
          { name: 'skills', type: 'string[]', required: true, description: 'Canonical skill tokens (non-empty).', example: '["go","postgresql"]' },
          { name: 'location_preferences', type: 'object', description: 'Optional location block (`work_modes`, `remote`, `base`, `relocation`); omit or null to clear.' },
        ],
        curl: `curl -X PUT "${BASE_URL}/me/profile" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"specializations":["backend"],"skills":["go","postgresql"]}'`,
        responseExample: `{ "data": { "specializations": ["backend"], "skills": ["go", "postgresql"], "location_preferences": null } }`,
      },
      {
        method: 'DELETE',
        path: '/me/profile',
        auth: 'cookie',
        summary: 'Clear your profile (idempotent).',
        description: 'Returns `204 No Content`.',
        curl: `curl -X DELETE "${BASE_URL}/me/profile" -b cookies.txt`,
      },
      {
        method: 'GET',
        path: '/me/profile/verdict',
        auth: 'cookie',
        summary: 'Market-coverage verdict for your profile skills.',
        description:
          'How many of your selected role’s open vacancies your skills reach, and ' +
          'which missing skill unlocks the most. The role is the request’s facet ' +
          'params (defaulting to your specializations). No profile is a `404`; `503` ' +
          'when search is unavailable.',
        query: [
          { name: '(any search filter)', type: 'string', description: 'Any search facet param scopes the role (the `skills` facet is ignored; your profile skills are the measured set).', example: 'category=backend' },
        ],
        curl: `curl "${BASE_URL}/me/profile/verdict" -b cookies.txt`,
        responseExample: `{
  "data": {
    "total": 1820,
    "covered": 1400,
    "coverage_percent": 77,
    "gaps": [ { "name": "kubernetes", "new_vacancies": 120, "unlock_percent": 7 } ],
    "skills": [ { "...": "..." } ],
    "coherence_percent": 64,
    "bundles": [ { "...": "..." } ]
  }
}`,
      },
      {
        method: 'GET',
        path: '/me/profile/ats-report',
        auth: 'cookie',
        summary: 'CV ATS-readiness report (deterministic + any cached LLM review).',
        description:
          'Scores your stored CV’s structure and its keyword match against the ' +
          'selected role. `has_cv` is false when no CV is stored; no profile is a ' +
          '`404`; `503` when search is unavailable.',
        curl: `curl "${BASE_URL}/me/profile/ats-report" -b cookies.txt`,
        responseExample: `{
  "data": {
    "has_cv": true,
    "report": {
      "overall": 78,
      "potential": 90,
      "categories": [ { "id": "structure", "label": "Structure", "score": 18, "max": 20, "items": [ { "...": "..." } ] } ],
      "strong_keywords": ["go", "postgresql"],
      "recommended_keywords": ["kubernetes"],
      "reviewed": false
    }
  }
}`,
      },
      {
        method: 'POST',
        path: '/me/profile/ats-report',
        auth: 'cookie',
        summary: 'Run the optional LLM qualitative ATS review and cache it.',
        description:
          'Runs the LLM review over your stored CV and folds it into the report ' +
          '(`reviewed: true`). Best-effort: an unconfigured or failing LLM returns the ' +
          'deterministic report (200).',
        curl: `curl -X POST "${BASE_URL}/me/profile/ats-report" -b cookies.txt`,
        responseExample: `{ "data": { "has_cv": true, "report": { "overall": 78, "reviewed": true, "...": "..." } } }`,
      },
      {
        method: 'POST',
        path: '/me/resume/extract',
        auth: 'cookie',
        summary: 'Extract a structured profile from an uploaded résumé (no LLM).',
        description:
          'Accepts a PDF (`multipart/form-data` field `file`) or plain text ' +
          '(`application/json` `{ "text": ... }`). Returns canonical skill slugs, the ' +
          'categories it spans, and the resolved seniority (omitted when unresolved). ' +
          'When storage is configured it also stores the résumé once.',
        body: [
          { name: 'text', type: 'string', description: 'Résumé plain text (JSON path); or send a PDF as multipart field `file`.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/resume/extract" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"text":"Senior Go engineer, 6 years..."}'`,
        responseExample: `{ "data": { "skills": ["go", "postgresql"], "categories": ["backend"], "seniority": "senior" } }`,
      },
      {
        method: 'PUT',
        path: '/me/resume',
        auth: 'cookie',
        summary: 'Store or replace your résumé.',
        description:
          'Accepts a PDF (multipart `file`) or JSON `{ "text": ... }`. Returns the ' +
          'résumé metadata. `501` when object storage is unconfigured.',
        body: [
          { name: 'text', type: 'string', description: 'Résumé plain text (JSON path); or send a PDF as multipart field `file`.' },
        ],
        curl: `curl -X PUT "${BASE_URL}/me/resume" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"text":"Senior Go engineer, 6 years..."}'`,
        responseExample: `{ "data": { "enabled": true, "present": true, "uploaded_at": "2026-06-19T10:00:00Z", "structured": null } }`,
      },
      {
        method: 'GET',
        path: '/me/resume',
        auth: 'cookie',
        summary: 'Your résumé status (enabled / present / uploaded_at).',
        description:
          'Always `200`: unconfigured storage or no résumé is a normal state. ' +
          '`structured` carries the read-only structured résumé, or null when none is ' +
          'current.',
        curl: `curl "${BASE_URL}/me/resume" -b cookies.txt`,
        responseExample: `{ "data": { "enabled": true, "present": true, "uploaded_at": "2026-06-19T10:00:00Z", "structured": { "...": "..." } } }`,
      },
      {
        method: 'DELETE',
        path: '/me/resume',
        auth: 'cookie',
        summary: 'Delete your stored résumé.',
        description: 'Returns `204 No Content`. `501` when object storage is unconfigured.',
        curl: `curl -X DELETE "${BASE_URL}/me/resume" -b cookies.txt`,
      },
    ],
  },
  {
    title: 'Activity & shared boards',
    intro:
      'Two public reads — the catalogue-activity time series and a shared saved-' +
      'search “board” by slug — plus the session-only publish/unpublish actions that ' +
      'turn one of your saved searches into such a board. A published board exposes ' +
      'no owner identity.',
    endpoints: [
      {
        method: 'GET',
        path: '/stats/jobs-activity',
        auth: 'none',
        summary: 'Public time series of added vs. removed vacancies per period.',
        description:
          'Aggregated to the requested granularity over a date range; the series is ' +
          'dense (missing periods are 0). Defaults: `granularity=day`, `to`=today, ' +
          '`from` a per-granularity window before `to`. An unknown granularity or a ' +
          'range over 4000 days is a `400`.',
        query: [
          { name: 'granularity', type: 'string', description: 'One of `day`, `week`, `month` (default `day`).', example: 'week' },
          { name: 'from', type: 'string (YYYY-MM-DD)', description: 'Start date (UTC). Defaults to a per-granularity window before `to`.', example: '2026-01-01' },
          { name: 'to', type: 'string (YYYY-MM-DD)', description: 'End date (UTC). Defaults to today.', example: '2026-06-30' },
        ],
        curl: `curl "${BASE_URL}/stats/jobs-activity?granularity=week"`,
        responseExample: `{
  "data": [
    { "period": "2026-06-01", "added": 120, "removed": 40 },
    { "period": "2026-06-08", "added": 98, "removed": 55 }
  ],
  "meta": { "granularity": "week", "from": "2025-06-09", "to": "2026-06-08" }
}`,
      },
      {
        method: 'GET',
        path: '/boards/{slug}',
        auth: 'none',
        summary: 'A shared saved-search board by its public slug.',
        description:
          'Public, no owner-scoping — returns only display fields (`name`, the ' +
          'canonical filter `query`, and an optional `author_label`). An unknown or ' +
          'unshared slug is a `404`.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The board public slug.', example: 'senior-go-remote-3f9a' }],
        curl: `curl "${BASE_URL}/boards/senior-go-remote-3f9a"`,
        responseExample: `{ "data": { "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote", "author_label": "Jane D." } }`,
      },
      {
        method: 'POST',
        path: '/me/searches/{id}/share',
        auth: 'cookie',
        summary: 'Publish one of your saved searches as a public board.',
        description:
          'Mints (or keeps) the board slug and sets the optional author label. Owner-' +
          'scoped; a missing/non-owned id is a `404`. Returns the saved search, now ' +
          'carrying `public_slug`.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
        body: [
          { name: 'author_label', type: 'string', description: 'Label shown on the board; blank/omitted renders it anonymously.', example: 'Jane D.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/searches/2/share" \\
  -H 'Content-Type: application/json' -b cookies.txt \\
  -d '{"author_label":"Jane D."}'`,
        responseExample: `{ "data": { "id": 2, "name": "Senior Go remote", "query": "q=go&seniority=senior&work_mode=remote", "public_slug": "senior-go-remote-3f9a", "author_label": "Jane D." } }`,
      },
      {
        method: 'DELETE',
        path: '/me/searches/{id}/share',
        auth: 'cookie',
        summary: 'Make a shared board private again.',
        description: 'Owner-scoped and idempotent (already-private is a no-op). Returns `204 No Content`.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The saved-search id.', example: '2' }],
        curl: `curl -X DELETE "${BASE_URL}/me/searches/2/share" -b cookies.txt`,
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
