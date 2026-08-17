// The freehire public API, described as data. This single module is the source
// of truth for BOTH the rendered /docs/api page and the generated docs/API.md
// (web/scripts/gen-api-docs.mjs), so the two can never drift. The job-search
// filter vocabulary is NOT duplicated here — it lives in ./filters, derived from
// the generated contracts so it stays in lock-step with the Go StringFacets.

/** Production base URL for every path below. */
export const BASE_URL = 'https://freehire.me/api/v1';

/** Authentication requirement for an endpoint, rendered as a badge. */
export type Auth = 'none' | 'cookie-or-key' | 'cookie' | 'moderator' | 'extension';

/** Human-readable label for an auth level. */
export const AUTH_LABELS: Record<Auth, string> = {
  none: 'Public',
  'cookie-or-key': 'Session or API key',
  cookie: 'Session only',
  moderator: 'Moderator',
  extension: 'Browser extension only',
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
  {
    title: 'What is not here',
    paragraphs: [
      'This reference covers every endpoint you can call. A handful are ' +
        'deliberately left out because calling them directly is meaningless: the ' +
        'Gmail consent redirect (`/me/gmail/connect` and its callback), which only ' +
        'a browser can complete; the Telegram bot webhook; the browser-tool ' +
        'websocket relay; and the sitemap-cursor helpers behind `/sitemap.xml`.',
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
        path: '/agent/jobs/search',
        auth: 'none',
        summary: 'Search with full descriptions, for programmatic/agent consumers.',
        description:
          'Same query and filters as `/jobs/search`, but each result carries the ' +
          '`description` in full (verbatim from the database) instead of the truncated ' +
          'index preview. Use `description_format` to choose how it is rendered.',
        filterable: true,
        query: [
          { name: 'q', type: 'string', description: 'Full-text query over title, company, and description.', example: 'golang' },
          { name: 'description_format', type: 'string', description: 'One of `html` (default, verbatim), `text` (tags stripped), `markdown` (HTML converted to Markdown). Unknown values fall back to `html`.', example: 'markdown' },
          { name: 'sort', type: 'string', description: 'One of `created_at`, `posted_at`, `salary_min`, `salary_max`. Omit for relevance/newest.', example: 'posted_at' },
          { name: 'order', type: 'string', description: '`asc` or `desc` (default `desc`).', example: 'desc' },
          { name: 'limit', type: 'integer', description: 'Page size, 1–100.', example: '20' },
          { name: 'offset', type: 'integer', description: 'Rows to skip; `offset + limit` ≤ 10000.', example: '0' },
        ],
        curl: `curl "${BASE_URL}/agent/jobs/search?q=golang&work_mode=remote&description_format=markdown"`,
        responseExample: `{
  "data": [ { "public_slug": "...", "title": "Senior Go Engineer", "description": "## About the role\\n...", "...": "..." } ],
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
      'deterministic (no LLM); the match-analysis endpoints run the LLM chain and cost AI ' +
      'credits. All take the same facet filter params as search where they narrow a ' +
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
        path: '/jobs/{slug}/match-analysis',
        auth: 'cookie-or-key',
        summary: 'The cached AI match analysis for the job (never runs the LLM).',
        description:
          'Returns the cached analysis, flagged `stale` when your CV or the job ' +
          'changed since it was computed, or a null analysis when none is cached. ' +
          '`has_cv` is false when you have no stored CV. `credits` reports your AI-points ' +
          'balance and when it resets.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl "${BASE_URL}/jobs/<slug>/match-analysis" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
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
    "credits": { "remaining": 17, "resets_at": "2026-08-01T00:00:00Z" }
  }
}`,
      },
      {
        method: 'POST',
        path: '/jobs/{slug}/match-analysis',
        auth: 'cookie-or-key',
        summary: 'Run the three-stage AI match analysis and cache it.',
        description:
          'Runs the match prompt-chain over your stored CV and the job, caches the ' +
          'result, and returns it fresh (no `credits` on this response). Analysing a new ' +
          'job costs one AI credit; if you have none left it is a `402`, and recomputing ' +
          'an already-analyzed job is free. `has_cv` is false when no CV is stored; a ' +
          'failing or unconfigured LLM returns a null analysis (200).',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/match-analysis" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
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
        path: '/jobs/{slug}/match-analysis/stream',
        auth: 'cookie-or-key',
        summary: 'Run the match analysis over Server-Sent Events.',
        description:
          'The same three-stage chain as `POST /jobs/{slug}/match-analysis`, streamed as SSE ' +
          '(`text/event-stream`) rather than a single JSON body. Each event’s `kind` ' +
          'is one of `stage_start`, `stage_done`, `thinking`, `requirements`, ' +
          '`dimensions`, `final`; the `final` event carries the completed `analysis` ' +
          '(the same shape as the match-analysis endpoints). Not a JSON endpoint.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -N "${BASE_URL}/jobs/<slug>/match-analysis/stream" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `data: {"kind":"stage_start","stage":1,"label":"Extracting requirements"}

data: {"kind":"requirements","requirements":[ { "...": "..." } ]}

data: {"kind":"final","analysis":{"overall_score":82,"verdict":"Strong Fit","...":"..."}}`,
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
          { name: 'industries', type: 'string', description: 'Curated industry. Matches a company through its curated industries **or** the equivalent job-derived domain. Repeatable.', example: 'fintech' },
          { name: 'domains', type: 'string', description: 'Raw job-derived domain, including values no industry names (`other`, `media`, `mobility`). Repeatable.', example: 'fintech' },
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
        description:
          'Send `applied_on` to record an application on the day it was actually sent — ' +
          'importing a history, or correcting a date already stored. A date you state ' +
          'overrides one we inferred. The day is stored at noon UTC, because you are stating ' +
          'a day and midnight reads as the previous date west of Greenwich. A date in the ' +
          'future, older than a year, or not a calendar date, is a 400. Without a body the ' +
          'application is stamped now, as before.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        body: [
          {
            name: 'applied_on',
            type: 'string',
            description: 'The day the application was sent (`YYYY-MM-DD`). Defaults to today.',
            example: '2026-07-27',
          },
        ],
        curl: `curl -X POST "${BASE_URL}/jobs/<slug>/apply" \\
  -H "Authorization: Bearer $FREEHIRE_API_KEY" -H 'Content-Type: application/json' \\
  -d '{"applied_on":"2026-07-27"}'`,
        responseExample: `{ "data": { "job_id": 42, "applied_at": "2026-07-27T12:00:00Z" } }`,
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
          '`preparing`, `applied`, `screening`, `responded`, `interview`, `offer`, `accepted`, ' +
          '`rejected`, `withdrawn`, `expired` (an unknown value is a 400). `expired` is the ' +
          'outcome for an application nobody answered; nothing sets it for you.',
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
          'Each item carries a card of the job with your interaction timestamps alongside ' +
          'it — what a list row draws: slug, title, company, closed_at, the stated facets, ' +
          'skills, collections, posted_at, and a `blurb` already cut to length. It does NOT ' +
          'carry the description; the full job view is on `GET /me/tracking/:slug`. ' +
          '`meta.counts` gives the per-filter totals for tab badges. Closed jobs stay ' +
          'listed so your history never shrinks.',
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
        summary: 'Jobs you have run the AI match analysis on.',
        description:
          'Newest first, closed jobs included (with `closed: true`). Each item carries the ' +
          'overall score and verdict; `stale` marks an analysis whose CV, job, or model has ' +
          'changed since. `meta.credits` reports your AI-points balance. Never runs the LLM.',
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
  "meta": { "credits": { "remaining": 17, "resets_at": "2026-08-01T00:00:00Z" } }
}`,
      },
      {
        method: 'GET',
        path: '/me/credits',
        auth: 'cookie-or-key',
        summary: 'Your current AI-credits balance.',
        description:
          'The points left this month (`remaining`) and when the monthly grant renews ' +
          '(`resets_at`). AI credits are spent on the match analysis (1) and CV tailoring (3), ' +
          'topped up by the monthly grant and by accepted board contributions. Never runs the LLM.',
        curl: `curl "${BASE_URL}/me/credits" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": { "remaining": 17, "resets_at": "2026-08-01T00:00:00Z" }
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
        summary: 'Your application-pipeline snapshot (counts per stage).',
        description:
          'The total application count and the count at each stage, aggregated server-side ' +
          'over all of your applications. Every stage of the vocabulary is present, zero ' +
          'included, and the counts always sum to `applications`. An application with ' +
          '`applied_at` set but no explicit stage counts as `applied`.',
        curl: `curl "${BASE_URL}/me/tracking/pipeline" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": {
    "applications": 12,
    "stages": {
      "applied": 4,
      "screening": 2,
      "responded": 1,
      "interview": 2,
      "offer": 1,
      "accepted": 1,
      "rejected": 1,
      "withdrawn": 0
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
      {
        method: 'GET',
        path: '/me/timeline',
        auth: 'cookie-or-key',
        summary: 'What happened to your applications over a date range.',
        description:
          'The application-event ledger as a dated series, oldest first: applications ' +
          'sent, employer replies, follow-ups and stage changes. `occurred_at` is an ' +
          'instant, not a day — which day it falls on depends on your timezone, so ' +
          'group it client-side. `observed` says whether the date came from a source ' +
          'other than you: mail-derived events carry a date the employer set, while a ' +
          'stage you set yourself is dated from when you recorded it. `email_id` and ' +
          '`email_subject` are present only while the message exists; deleting it hides ' +
          'the content and leaves the event standing. Both bounds are required and may ' +
          'span at most 366 days.',
        query: [
          { name: 'from', type: 'string (RFC3339)', required: true, description: 'Lower bound, inclusive. Use `Z`, or percent-encode a numeric offset — a bare `+` decodes as a space in a query string and will be rejected.', example: '2026-08-01T00:00:00Z' },
          { name: 'to', type: 'string (RFC3339)', required: true, description: 'Upper bound, inclusive.', example: '2026-08-31T23:59:59Z' },
        ],
        curl: `curl "${BASE_URL}/me/timeline?from=2026-08-01T00:00:00Z&to=2026-08-31T23:59:59Z" -H "Authorization: Bearer $FREEHIRE_API_KEY"`,
        responseExample: `{
  "data": [
    {
      "id": 7,
      "kind": "employer_reply",
      "signal": "interview_invitation",
      "source": "mail_gmail",
      "observed": true,
      "occurred_at": "2026-08-13T09:41:00Z",
      "company_slug": "derq",
      "role_title": "Senior Go Engineer",
      "application_id": 31,
      "job_slug": "senior-go-engineer-derq-1a2b",
      "email_id": 42,
      "email_subject": "Invitation to interview"
    },
    {
      "id": 8,
      "kind": "stage_set",
      "signal": "screening",
      "source": "user",
      "observed": false,
      "occurred_at": "2026-08-13T21:15:00Z",
      "company_slug": "linear",
      "application_id": 33
    }
  ],
  "meta": { "from": "2026-08-01T00:00:00Z", "to": "2026-08-31T23:59:59Z", "count": 2 }
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
        summary: 'Resolve a report, optionally closing the job and telling the reporter.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
        body: [
          { name: 'close_job', type: 'boolean', description: 'Soft-close the reported job.', example: 'true' },
          {
            name: 'note',
            type: 'string',
            description: 'What you did about the report. Emailed to the reporter verbatim when notify_reporter is set, and stored as the review reason either way.',
            example: 'Fixed — the job is now marked hybrid',
          },
          {
            name: 'notify_reporter',
            type: 'boolean',
            description: 'Email the reporter this decision. Defaults to false when omitted.',
            example: 'true',
          },
        ],
        curl: `curl -X POST "${BASE_URL}/reports/3/resolve" \\
  -H "Authorization: Bearer $MODERATOR_API_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"close_job":true,"note":"Fixed — the job is now marked hybrid","notify_reporter":true}'`,
        responseExample: `{ "data": { "id": 3, "status": "resolved", "notified": true } }`,
      },
      {
        method: 'POST',
        path: '/reports/{id}/dismiss',
        auth: 'moderator',
        summary: 'Dismiss a report with a reason, optionally telling the reporter.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The report id.', example: '3' }],
        body: [
          {
            name: 'reason',
            type: 'string',
            description: 'Why nothing changed. Emailed to the reporter verbatim when notify_reporter is set.',
            example: 'not an issue',
          },
          {
            name: 'notify_reporter',
            type: 'boolean',
            description: 'Email the reporter this decision. Defaults to false when omitted.',
            example: 'true',
          },
        ],
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
  {
    title: 'Account, credits & extension',
    intro:
      'The rest of the account surface: the password, deleting the account, the ' +
      'AI-credit ledger, and the two endpoints the browser extension runs on. ' +
      'Password and deletion are session-only — an API key must not be able to ' +
      'change or destroy the credential it would outlive.',
    endpoints: [
      {
        method: 'POST',
        path: '/me/password',
        auth: 'cookie',
        summary: 'Change a known password.',
        description:
          'Revokes every session; your own cookie is re-issued at the new ' +
          'generation, so you stay signed in and everyone else is signed out.',
        body: [
          { name: 'current_password', type: 'string', required: true, description: 'Your existing password.' },
          { name: 'new_password', type: 'string', required: true, description: 'The replacement.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/password" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"current_password":"…","new_password":"…"}'`,
        responseExample: `{ "data": { "ok": true } }`,
      },
      {
        method: 'DELETE',
        path: '/me',
        auth: 'cookie',
        summary: 'Delete your account and everything under it.',
        curl: `curl -X DELETE "${BASE_URL}/me" -b cookies.txt`,
      },
      {
        method: 'GET',
        path: '/me/credits/history',
        auth: 'cookie-or-key',
        summary: 'Your AI-credit ledger, newest first.',
        description: 'Each debit is labelled with what it bought — the job a match analysis was run on, the vacancy a CV was tailored for — rather than an opaque reference.',
        query: [
          { name: 'limit', type: 'integer', description: 'Page size.' },
          { name: 'offset', type: 'integer', description: 'Page offset.' },
        ],
        curl: `curl "${BASE_URL}/me/credits/history" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "delta": -1, "reason": "match_analysis", "label": "Senior Backend Engineer at Acme", "created_at": "2026-07-28T09:00:00Z" } ] }`,
      },
      {
        method: 'GET',
        path: '/me/tracking/{slug}',
        auth: 'cookie-or-key',
        summary: 'One tracked application, with the mail linked to it and its history.',
        description:
          '`events` is the application\'s ledger, newest first — the apply, employer ' +
          'replies, follow-ups, stage changes, scheduled interviews — in the shape ' +
          '`GET /me/timeline` serves, bounded at 100 and empty when nothing has happened ' +
          'yet. This read also carries the full job view; the listing carries only a card. ' +
          'A slug you do not track is a 404.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl "${BASE_URL}/me/tracking/senior-backend-engineer-acme-1a2b" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{
  "data": {
    "slug": "senior-backend-engineer-acme-1a2b",
    "stage": "interview",
    "applied_at": "2026-07-24T09:00:00Z",
    "emails": [ { "id": 4821, "subject": "Interview for …", "status_signal": "interview_invitation" } ],
    "events": [
      { "id": 991, "kind": "employer_reply", "signal": "interview_invitation", "source": "mail_gmail", "observed": true, "occurred_at": "2026-07-29T11:04:00Z" },
      { "id": 802, "kind": "applied", "source": "user", "observed": false, "occurred_at": "2026-07-24T09:00:00Z" }
    ]
  }
}`,
      },
      {
        method: 'GET',
        path: '/me/tracking/dismissed',
        auth: 'cookie-or-key',
        summary: 'The slugs you dismissed, so a client can hide them.',
        curl: `curl "${BASE_URL}/me/tracking/dismissed" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ "junior-php-developer-acme-9z8y" ] }`,
      },
      {
        method: 'POST',
        path: '/me/match-text',
        auth: 'cookie-or-key',
        summary: 'Score any job text against your profile.',
        description:
          'The same deterministic skill coverage as `GET /jobs/{slug}/match`, but ' +
          'for a page that need not be in the catalogue — this is what lets the ' +
          'extension show a match on any job page. A caller with no profile is a 404. ' +
          'No LLM, no credits.',
        body: [
          { name: 'title', type: 'string', required: true, description: 'The posting title.' },
          { name: 'text', type: 'string', required: true, description: 'The scraped posting text.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/match-text" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"title":"Senior Backend Engineer","text":"We are looking for …"}'`,
        responseExample: `{ "data": { "score": 68, "matched": ["go","postgresql"], "missing": ["kubernetes"] } }`,
      },
      {
        method: 'GET',
        path: '/me/autofill-profile',
        auth: 'cookie-or-key',
        summary: 'Your contact details in canonical autofill fields.',
        description: "Projected from your CV header, with the account email as a fallback — what the extension writes into application forms.",
        curl: `curl "${BASE_URL}/me/autofill-profile" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "first_name": "Ilya", "last_name": "Strelov", "email": "you@example.com", "phone": "+1…", "linkedin": "https://linkedin.com/in/you", "portfolio": "https://ilya.dev" } }`,
      },
      {
        method: 'POST',
        path: '/me/autofill/run',
        auth: 'extension',
        summary: 'Drive your own browser to fill an application form.',
        description:
          'Runs the autofill agent over the browser-tool wire — your extension on ' +
          'one end, the agent on the other, routed strictly within your own channel. ' +
          "Requires the extension to be connected, and authenticates only as the " +
          'extension itself — a session cookie or an API key gets a 403, since this ' +
          'call writes into whatever form your browser has open, not just reads it.',
        curl: `curl -X POST "${BASE_URL}/me/autofill/run" -H "Authorization: Bearer <extension session token>"`,
        responseExample: `{ "data": { "filled": 11, "skipped": 2 } }`,
      },
    ],
  },
  {
    title: 'Link intake & discovery',
    intro:
      'Turning a URL in the wild into something freehire knows about. One intake ' +
      'sequence serves every surface — the website, the bot, the extension and the ' +
      'CLI — deliberately, so a pasted link gets the same answer everywhere.',
    endpoints: [
      {
        method: 'GET',
        path: '/jobs/find',
        auth: 'none',
        summary: 'Resolve a posting URL to a catalogue slug.',
        description:
          'Answers `{"data": null}` when the posting cannot be identified, rather ' +
          'than 404 — "we do not carry this" is a normal answer here. Used by the ' +
          'extension to tell whether the page you are on is a job we already have.',
        query: [{ name: 'url', type: 'string', required: true, description: "The posting's URL in the wild.", example: 'https://boards.greenhouse.io/acme/jobs/123' }],
        curl: `curl "${BASE_URL}/jobs/find?url=https%3A%2F%2Fboards.greenhouse.io%2Facme%2Fjobs%2F123"`,
        responseExample: `{ "data": { "public_slug": "senior-backend-engineer-acme-1a2b" } }`,
      },
      {
        method: 'POST',
        path: '/jobs/resolve',
        auth: 'cookie-or-key',
        summary: 'Hand freehire a link: import the posting, and the board behind it.',
        description:
          'Five outcomes in one shape, distinguished by status: **200 found** — the ' +
          'catalogue already carries the vacancy. Either the URL itself is stored (nothing ' +
          'is fetched or written), or the page turned out to be a second copy of a posting ' +
          'we already carry under another source, in which case the answer names the one we ' +
          'had; **201 tracked** ' +
          '— we crawl that board already and the posting just had not landed, so it ' +
          'was imported now; **201 imported** — imported, and its board queued for ' +
          'onboarding; **201 review** — imported, but its careers site names no board ' +
          'we can crawl, so the link went to manual triage; **202 queued** — nothing ' +
          'could read the page, so the link went to manual triage. `company_slug` is ' +
          'returned whenever the catalogue already carries the employer, through any ' +
          'source — independent of what became of the board. Rate-limited, because it ' +
          'makes the server fetch a URL you chose. A URL that is not http(s) is a 422.',
        body: [
          { name: 'url', type: 'string', required: true, description: 'The job page to resolve.' },
          { name: 'surface', type: 'string', description: 'Where the link came from (website, extension, cli, bot) — recorded, not validated against a whitelist.' },
        ],
        curl: `curl -X POST "${BASE_URL}/jobs/resolve" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"url":"https://boards.greenhouse.io/acme/jobs/123","surface":"cli"}'`,
        responseExample: `{ "data": { "status": "tracked", "public_slug": "senior-backend-engineer-acme-1a2b", "company_slug": "acme" } }`,
      },
      {
        method: 'GET',
        path: '/me/contributions',
        auth: 'cookie-or-key',
        summary: 'The boards you contributed, and their onboarding state.',
        curl: `curl "${BASE_URL}/me/contributions" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "url": "https://boards.greenhouse.io/acme", "source": "greenhouse", "board": "acme", "state": "onboarded", "created_at": "2026-07-20T12:00:00Z" } ] }`,
      },
      {
        method: 'POST',
        path: '/me/contributions',
        auth: 'cookie-or-key',
        summary: 'Contribute a board by link.',
        body: [{ name: 'url', type: 'string', required: true, description: "A vacancy or a company's careers page." }],
        curl: `curl -X POST "${BASE_URL}/me/contributions" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"url":"https://boards.greenhouse.io/acme"}'`,
        responseExample: `{ "data": { "source": "greenhouse", "board": "acme", "state": "pending" } }`,
      },
    ],
  },
  {
    title: 'Votes, notifications & discussions',
    intro:
      'The lighter per-user surfaces: a vote on a job or company, the account-level ' +
      'notification rule (gates the saved-job apply reminder and both lifecycle nudges), ' +
      'and the public discussion threads. Votes and notification settings take a key; ' +
      'posting to a thread is browser-owned.',
    endpoints: [
      {
        method: 'POST',
        path: '/jobs/{slug}/vote',
        auth: 'cookie-or-key',
        summary: 'Vote on a job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        body: [{ name: 'value', type: 'integer', required: true, description: '`1` or `-1`.' }],
        curl: `curl -X POST "${BASE_URL}/jobs/senior-backend-engineer-acme-1a2b/vote" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"value":1}'`,
        responseExample: `{ "data": { "score": 14, "my_vote": 1 } }`,
      },
      {
        method: 'DELETE',
        path: '/jobs/{slug}/vote',
        auth: 'cookie-or-key',
        summary: 'Clear your vote on a job.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug`.' }],
        curl: `curl -X DELETE "${BASE_URL}/jobs/senior-backend-engineer-acme-1a2b/vote" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "score": 13, "my_vote": 0 } }`,
      },
      {
        method: 'POST',
        path: '/companies/{slug}/vote',
        auth: 'cookie-or-key',
        summary: 'Vote on a company.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.' }],
        body: [{ name: 'value', type: 'integer', required: true, description: '`1` or `-1`.' }],
        curl: `curl -X POST "${BASE_URL}/companies/acme/vote" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"value":1}'`,
        responseExample: `{ "data": { "score": 42, "my_vote": 1 } }`,
      },
      {
        method: 'DELETE',
        path: '/companies/{slug}/vote',
        auth: 'cookie-or-key',
        summary: 'Clear your vote on a company.',
        pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.' }],
        curl: `curl -X DELETE "${BASE_URL}/companies/acme/vote" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "score": 41, "my_vote": 0 } }`,
      },
      {
        method: 'GET',
        path: '/me/notification-settings',
        auth: 'cookie',
        summary: 'Your notification rule (gates saved-job reminders and both lifecycle nudges).',
        curl: `curl "${BASE_URL}/me/notification-settings" -b cookies.txt`,
        responseExample: `{ "data": { "enabled": true, "channels": ["email"] } }`,
      },
      {
        method: 'PUT',
        path: '/me/notification-settings',
        auth: 'cookie',
        summary: 'Change your notification rule.',
        body: [
          { name: 'enabled', type: 'boolean', description: 'Turn notifications on or off.' },
          { name: 'channels', type: 'string[]', description: 'Delivery channels: `email`, `telegram`.' },
        ],
        curl: `curl -X PUT "${BASE_URL}/me/notification-settings" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"enabled":true,"channels":["email"]}'`,
        responseExample: `{ "data": { "enabled": true, "channels": ["email"] } }`,
      },
      {
        method: 'GET',
        path: '/threads',
        auth: 'none',
        summary: 'Public discussion threads.',
        query: [
          { name: 'limit', type: 'integer', description: 'Page size.' },
          { name: 'offset', type: 'integer', description: 'Page offset.' },
        ],
        curl: `curl "${BASE_URL}/threads?limit=20"`,
        responseExample: `{ "data": [ { "id": 31, "title": "Anyone interviewed at Acme lately?", "replies": 12, "created_at": "2026-07-25T10:00:00Z" } ], "meta": { "total": 214 } }`,
      },
      {
        method: 'GET',
        path: '/threads/count',
        auth: 'none',
        summary: 'How many threads there are.',
        curl: `curl "${BASE_URL}/threads/count"`,
        responseExample: `{ "data": { "count": 214 } }`,
      },
      {
        method: 'GET',
        path: '/threads/{id}',
        auth: 'none',
        summary: 'One thread with its replies.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
        curl: `curl "${BASE_URL}/threads/31"`,
        responseExample: `{ "data": { "id": 31, "title": "Anyone interviewed at Acme lately?", "body": "…", "replies": [ { "id": 88, "body": "…", "created_at": "2026-07-25T12:00:00Z" } ] } }`,
      },
      {
        method: 'POST',
        path: '/threads',
        auth: 'cookie',
        summary: 'Start a thread.',
        body: [
          { name: 'title', type: 'string', required: true, description: 'Thread title.' },
          { name: 'body', type: 'string', required: true, description: 'Opening post.' },
        ],
        curl: `curl -X POST "${BASE_URL}/threads" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Anyone interviewed at Acme lately?","body":"…"}'`,
        responseExample: `{ "data": { "id": 31 } }`,
      },
      {
        method: 'POST',
        path: '/threads/{id}/replies',
        auth: 'cookie',
        summary: 'Reply to a thread.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
        body: [{ name: 'body', type: 'string', required: true, description: 'Your reply.' }],
        curl: `curl -X POST "${BASE_URL}/threads/31/replies" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"body":"Two rounds, both technical."}'`,
        responseExample: `{ "data": { "id": 88 } }`,
      },
      {
        method: 'POST',
        path: '/threads/{id}/close',
        auth: 'moderator',
        summary: 'Close a thread.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The thread id.' }],
        curl: `curl -X POST "${BASE_URL}/threads/31/close" -b cookies.txt`,
        responseExample: `{ "data": { "id": 31, "closed": true } }`,
      },
    ],
  },
  {
    title: 'Market insights & stats',
    intro:
      'Aggregate market intelligence, all public and all unauthenticated. Every ' +
      'figure is served from a precomputed rollup rather than counted live, and ' +
      'nothing here exposes a record-level field or a user identifier — these are ' +
      'counts over the catalogue, not a way to read it.',
    endpoints: [
      {
        method: 'GET',
        path: '/insights/roles',
        auth: 'none',
        summary: 'Roles (category × seniority) ranked by openings or growth.',
        query: [
          { name: 'country', type: 'string', description: 'Scope to one country (ISO code).', example: 'de' },
          { name: 'sort', type: 'string', description: 'Rank by open count or by growth.', example: 'growth' },
          { name: 'limit', type: 'integer', description: 'How many rows.' },
          { name: 'category', type: 'string', description: 'Scope to one job category.', example: 'backend' },
        ],
        curl: `curl "${BASE_URL}/insights/roles?country=de&sort=growth&limit=20"`,
        responseExample: `{ "data": [ { "category": "backend", "seniority": "senior", "open": 1840, "growth_pct": 12.4 } ] }`,
      },
      {
        method: 'GET',
        path: '/insights/skills',
        auth: 'none',
        summary: 'Skills ranked by openings or growth.',
        description: 'Scope by category or by country, but not both — the rollup behind it is single-dimensional, so asking for both is a 400.',
        query: [
          { name: 'category', type: 'string', description: 'Scope to one category.' },
          { name: 'country', type: 'string', description: 'Scope to one country.' },
          { name: 'sort', type: 'string', description: 'Rank by open count or growth.' },
          { name: 'limit', type: 'integer', description: 'How many rows.' },
        ],
        curl: `curl "${BASE_URL}/insights/skills?category=backend&sort=growth"`,
        responseExample: `{ "data": [ { "skill": "go", "open": 4210, "growth_pct": 8.1 } ] }`,
      },
      {
        method: 'GET',
        path: '/insights/salary',
        auth: 'none',
        summary: 'Salary bands by category, seniority and country.',
        query: [
          { name: 'category', type: 'string', description: 'Scope to one category.' },
          { name: 'seniority', type: 'string', description: 'Scope to one seniority.' },
          { name: 'country', type: 'string', description: 'Scope to one country.' },
          { name: 'limit', type: 'integer', description: 'How many rows.' },
        ],
        curl: `curl "${BASE_URL}/insights/salary?category=backend&seniority=senior"`,
        responseExample: `{ "data": [ { "category": "backend", "seniority": "senior", "currency": "USD", "p25": 130000, "p50": 165000, "p75": 200000, "samples": 412 } ] }`,
      },
      {
        method: 'GET',
        path: '/insights/velocity',
        auth: 'none',
        summary: 'How fast a slice of the market is hiring, over time.',
        query: [
          { name: 'granularity', type: 'string', description: 'Bucket size for the series.', example: 'week' },
          { name: 'from', type: 'string (date)', description: 'Start of the window.' },
          { name: 'to', type: 'string (date)', description: 'End of the window.' },
          { name: 'category', type: 'string', description: 'Scope to one category.' },
          { name: 'seniority', type: 'string', description: 'Scope to one seniority.' },
        ],
        curl: `curl "${BASE_URL}/insights/velocity?granularity=week&category=backend"`,
        responseExample: `{ "data": [ { "period": "2026-07-20", "added": 3120, "removed": 2740 } ] }`,
      },
      {
        method: 'GET',
        path: '/insights/companies',
        auth: 'none',
        summary: 'Companies ranked by hiring activity.',
        query: [
          { name: 'sort', type: 'string', description: 'Rank by openings or by growth.' },
          { name: 'min_open', type: 'integer', description: 'Ignore companies below this many open roles.' },
          { name: 'limit', type: 'integer', description: 'How many rows.' },
        ],
        curl: `curl "${BASE_URL}/insights/companies?sort=growth&min_open=10"`,
        responseExample: `{ "data": [ { "slug": "acme", "name": "Acme", "open": 87, "growth_pct": 21.0 } ] }`,
      },
      {
        method: 'GET',
        path: '/stats/facets',
        auth: 'none',
        summary: 'Facet distribution snapshot: countries, skills, seniority, work mode.',
        description: 'Served from the facet rollup rather than a live Meilisearch facet count, so a page can show "what\'s inside" without paying search for it.',
        curl: `curl "${BASE_URL}/stats/facets"`,
        responseExample: `{ "data": { "countries": [ { "value": "de", "count": 18420 } ], "skills": [ { "value": "go", "count": 4210 } ], "seniority": [ … ], "work_mode": [ … ] } }`,
      },
      {
        method: 'GET',
        path: '/stats/user-growth',
        auth: 'none',
        summary: 'Cumulative member growth per UTC day.',
        description: 'Aggregate-only — no user identifier is exposed.',
        curl: `curl "${BASE_URL}/stats/user-growth"`,
        responseExample: `{ "data": [ { "day": "2026-07-27", "total": 12840 } ] }`,
      },
      {
        method: 'GET',
        path: '/stats/engagement',
        auth: 'none',
        summary: 'Jobs saved, applied to and viewed across all users, plus CV and inbox usage.',
        curl: `curl "${BASE_URL}/stats/engagement"`,
        responseExample: `{ "data": { "saved": 41200, "applied": 18730, "viewed": 903400, "cvs_uploaded": 2140, "cvs_tailored": 860, "match_analyses": 5310, "inboxes_connected": 410, "saved_searches": 1290 } }`,
      },
      {
        method: 'GET',
        path: '/status',
        auth: 'none',
        summary: 'Ingest-fleet health, per provider.',
        description: 'Sanitized: which boards are healthy, cooled or failing, without error text or internal identifiers.',
        curl: `curl "${BASE_URL}/status"`,
        responseExample: `{ "data": { "providers": [ { "source": "greenhouse", "boards": 812, "failing": 14, "last_run_at": "2026-07-28T18:00:00Z" } ] } }`,
      },
    ],
  },
  {
    title: 'Employee referrals',
    intro:
      'Two sides of the same marketplace: an employee offers to refer into the ' +
      'company they work at (proof required, moderated), and a candidate asks one ' +
      'of that company\'s approved referrers for an intro. A candidate never sees ' +
      "who the referrer is, and a referrer sees the candidate's CV only through a " +
      'short-lived view. Moderation of offers is moderator-gated.',
    endpoints: [
      {
        method: 'POST',
        path: '/me/referrals/offers',
        auth: 'cookie-or-key',
        summary: 'Offer to refer into a company. Multipart — proof required.',
        description:
          'The proof (an offer letter, badge, anything showing you work there) is ' +
          'stored privately and shown only to a moderator; its storage key is never ' +
          'exposed on the wire. The offer is pending until decided.',
        body: [
          { name: 'company_slug', type: 'string (form field)', required: true, description: 'The company you can refer into.' },
          { name: 'linkedin_url', type: 'string (form field)', description: 'Your profile, for the moderator to check against.' },
          { name: 'proof', type: 'file (form field)', required: true, description: 'Evidence you work there.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/referrals/offers" \\
  -H "Authorization: Bearer fhk_…" \\
  -F company_slug=acme -F linkedin_url=https://linkedin.com/in/you -F proof=@badge.pdf`,
        responseExample: `{ "data": { "id": "b71e…", "company_slug": "acme", "company_name": "Acme", "status": "pending", "created_at": "2026-07-28T20:00:00Z" } }`,
      },
      {
        method: 'GET',
        path: '/me/referrals/offers',
        auth: 'cookie-or-key',
        summary: 'Your referral offers and their moderation state.',
        curl: `curl "${BASE_URL}/me/referrals/offers" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "id": "b71e…", "company_slug": "acme", "status": "approved", "decided_at": "2026-07-28T21:00:00Z" } ] }`,
      },
      {
        method: 'DELETE',
        path: '/me/referrals/offers/{id}',
        auth: 'cookie-or-key',
        summary: 'Withdraw an offer.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
        curl: `curl -X DELETE "${BASE_URL}/me/referrals/offers/b71e…" -H "Authorization: Bearer fhk_…"`,
      },
      {
        method: 'POST',
        path: '/me/referrals/requests',
        auth: 'cookie-or-key',
        summary: 'Ask a company\'s approved referrers for an intro.',
        description:
          'Validation failures answer 422; a company with no approved referrer ' +
          'answers 409; too many open requests answers 429.',
        body: [
          { name: 'company_slug', type: 'string', required: true, description: 'The company to ask.' },
          { name: 'job_id', type: 'integer', description: 'The specific opening, when there is one.' },
          { name: 'cv_kind', type: 'string', description: 'Which CV to attach: your stored one, or a built CV.' },
          { name: 'cv_id', type: 'string (uuid)', description: 'The built CV to attach, when `cv_kind` names one.' },
          { name: 'linkedin_url', type: 'string', description: 'Your profile.' },
          { name: 'contact_telegram', type: 'string', description: 'How the referrer reaches you.' },
          { name: 'contact_email', type: 'string', description: 'How the referrer reaches you.' },
          { name: 'note', type: 'string', description: 'A short message to the referrer.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/referrals/requests" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' \\
  -d '{"company_slug":"acme","cv_kind":"stored","contact_email":"you@example.com"}'`,
        responseExample: `{ "data": { "id": "c92f…", "company_slug": "acme", "status": "open", "created_at": "2026-07-28T20:05:00Z" } }`,
      },
      {
        method: 'GET',
        path: '/me/referrals/requests',
        auth: 'cookie-or-key',
        summary: 'The intros you asked for, and where they stand.',
        curl: `curl "${BASE_URL}/me/referrals/requests" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "id": "c92f…", "company_slug": "acme", "status": "contacted" } ] }`,
      },
      {
        method: 'GET',
        path: '/me/referrals/incoming',
        auth: 'cookie-or-key',
        summary: 'Requests waiting on you as an approved referrer.',
        curl: `curl "${BASE_URL}/me/referrals/incoming" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "id": "c92f…", "company_slug": "acme", "cv_kind": "stored", "status": "open", "note": "Backend, 8 years Go" } ] }`,
      },
      {
        method: 'GET',
        path: '/me/referrals/incoming/{id}/cv',
        auth: 'cookie-or-key',
        summary: "View the candidate's CV for one incoming request.",
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The request id.' }],
        curl: `curl "${BASE_URL}/me/referrals/incoming/c92f…/cv" -H "Authorization: Bearer fhk_…" -o candidate.pdf`,
      },
      {
        method: 'POST',
        path: '/me/referrals/incoming/{id}/resolve',
        auth: 'cookie-or-key',
        summary: 'Mark an incoming request contacted or declined.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The request id.' }],
        body: [{ name: 'status', type: 'string', required: true, description: '`contacted` or `declined`.', example: 'contacted' }],
        curl: `curl -X POST "${BASE_URL}/me/referrals/incoming/c92f…/resolve" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"status":"contacted"}'`,
        responseExample: `{ "data": { "id": "c92f…", "status": "contacted" } }`,
      },
      {
        method: 'GET',
        path: '/referrals/offers',
        auth: 'moderator',
        summary: 'The offer-moderation queue.',
        curl: `curl "${BASE_URL}/referrals/offers" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": [ { "id": "b71e…", "company_slug": "acme", "linkedin_url": "https://linkedin.com/in/you", "status": "pending" } ] }`,
      },
      {
        method: 'GET',
        path: '/referrals/offers/{id}/proof',
        auth: 'moderator',
        summary: "View an offer's proof document.",
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
        curl: `curl "${BASE_URL}/referrals/offers/b71e…/proof" -H "Authorization: Bearer fhk_…" -o proof.pdf`,
      },
      {
        method: 'POST',
        path: '/referrals/offers/{id}/decide',
        auth: 'moderator',
        summary: 'Approve or reject an offer.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The offer id.' }],
        body: [{ name: 'approve', type: 'boolean', required: true, description: 'True approves the referrer; false rejects the offer.' }],
        curl: `curl -X POST "${BASE_URL}/referrals/offers/b71e…/decide" \\
  -H "Authorization: Bearer fhk_…" -H 'Content-Type: application/json' -d '{"approve":true}'`,
        responseExample: `{ "data": { "id": "b71e…", "status": "approved", "decided_at": "2026-07-28T21:00:00Z" } }`,
      },
    ],
  },
  {
    title: 'CV builder & tailoring',
    intro:
      'Your CVs, and the tailoring flow that reframes one toward a single ' +
      'vacancy. Authoring is browser-owned — create, update and delete are ' +
      'session-only — while reading a CV, patching it, and rendering the PDF also ' +
      'accept an API key, because that is the half a tailoring agent drives. Every ' +
      'CV is owner-scoped: an id you do not own is a 404, not a 403.',
    endpoints: [
      {
        method: 'GET',
        path: '/me/cvs',
        auth: 'cookie',
        summary: 'List your CVs, without their documents.',
        curl: `curl "${BASE_URL}/me/cvs" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "created_at": "2026-07-01T10:00:00Z", "updated_at": "2026-07-20T18:30:00Z" } ] }`,
      },
      {
        method: 'POST',
        path: '/me/cvs',
        auth: 'cookie',
        summary: 'Create a CV, optionally seeded from your stored history.',
        body: [
          { name: 'title', type: 'string', description: 'Label; up to 200 characters.' },
          { name: 'template_id', type: 'string', description: 'Which template to render with.', example: 'classic' },
          { name: 'seed', type: 'boolean', description: 'Pre-fill from your parsed CV / experience bank instead of starting blank.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/cvs" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Backend engineer","seed":true}'`,
        responseExample: `{ "data": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "document": { "header": { … }, "experience": [ … ] } } }`,
      },
      {
        method: 'GET',
        path: '/me/cvs/{id}',
        auth: 'cookie-or-key',
        summary: 'One CV with its full document.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        curl: `curl "${BASE_URL}/me/cvs/0f2c…" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "id": "0f2c…", "title": "Backend engineer", "template_id": "classic", "agent_session_id": "s_9f…", "document": { … } } }`,
      },
      {
        method: 'PUT',
        path: '/me/cvs/{id}',
        auth: 'cookie',
        summary: 'Replace a CV — title, template and document.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        body: [
          { name: 'title', type: 'string', description: 'New label.' },
          { name: 'template_id', type: 'string', description: 'New template.' },
          { name: 'document', type: 'object', required: true, description: 'The whole CV document.' },
        ],
        curl: `curl -X PUT "${BASE_URL}/me/cvs/0f2c…" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"title":"Backend engineer","document":{…}}'`,
        responseExample: `{ "data": { "id": "0f2c…", "updated_at": "2026-07-28T20:10:00Z" } }`,
      },
      {
        method: 'PATCH',
        path: '/me/cvs/{id}',
        auth: 'cookie-or-key',
        summary: 'Apply a batch of edits, addressed by path.',
        description:
          'An edit is a `kind` (`set`, `insert`, `remove`, `move`) and a `path` into the CV — ' +
          '`summary`, `experience[2].bullets[1]`, `skills[0].items[3]`, `style.font_size`. ' +
          'Indices are 0-based. The whole batch applies or none of it does: an unknown path or ' +
          'an index past the end refuses the request without touching the CV. An API key edits ' +
          'as the tailoring agent, so the candidate\'s own fields are refused and a claim about ' +
          'them needs `evidence_id` from the experience bank. Every request lands in the CV\'s ' +
          'history and can be undone on its own.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        body: [
          { name: 'ops', type: 'object[]', required: true, description: 'The edits, applied in order.' },
          { name: 'ops[].kind', type: 'string', required: true, description: '`set`, `insert`, `remove` or `move`.', example: 'set' },
          { name: 'ops[].path', type: 'string', required: true, description: 'Where to edit.', example: 'experience[0].bullets[1]' },
          { name: 'ops[].value', type: 'any', description: 'The new content, for `set` and `insert`.' },
          { name: 'ops[].to', type: 'integer', description: "The element's new position, for `move`." },
          { name: 'ops[].evidence_id', type: 'string', description: 'The banked achievement this rests on.' },
          { name: 'note', type: 'string', description: 'One line on why, shown beside the entry in the history.' },
        ],
        curl: `curl -X PATCH "${BASE_URL}/me/cvs/0f2c…" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"ops":[{"kind":"set","path":"experience[0].bullets[1]","value":"Cut p99 checkout latency 40% by …","evidence_id":"a71f…"}]}'`,
        responseExample: `{ "data": { "id": "0f2c…", "document": { … } } }`,
      },
      {
        method: 'PUT',
        path: '/me/cvs/{id}/template',
        auth: 'cookie',
        summary: 'Switch the template only.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        body: [{ name: 'template_id', type: 'string', required: true, description: 'A template id from `/cv-templates`.' }],
        curl: `curl -X PUT "${BASE_URL}/me/cvs/0f2c…/template" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"template_id":"compact"}'`,
        responseExample: `{ "data": { "id": "0f2c…", "template_id": "compact" } }`,
      },
      {
        method: 'DELETE',
        path: '/me/cvs/{id}',
        auth: 'cookie',
        summary: 'Delete a CV.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        curl: `curl -X DELETE "${BASE_URL}/me/cvs/0f2c…" -b cookies.txt`,
      },
      {
        method: 'GET',
        path: '/me/cvs/{id}/pdf',
        auth: 'cookie-or-key',
        summary: 'Render the CV to PDF.',
        description: 'Answers 501 on a deployment with no typst binary configured; everything else still works.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        curl: `curl "${BASE_URL}/me/cvs/0f2c…/pdf" -H "Authorization: Bearer fhk_…" -o cv.pdf`,
      },
      {
        method: 'GET',
        path: '/cv-templates',
        auth: 'cookie',
        summary: 'The template gallery.',
        curl: `curl "${BASE_URL}/cv-templates" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": "classic", "name": "Classic" }, { "id": "compact", "name": "Compact" } ] }`,
      },
      {
        method: 'GET',
        path: '/cv-fonts',
        auth: 'cookie',
        summary: 'The typefaces a CV may use.',
        description:
          'Read this rather than hard-coding a list: an unregistered `document.style.font_family` is dropped on save and the CV renders in the template\'s own face. `note` names the familiar face the entry matches; `css` is a font stack for rendering a preview in a browser.',
        curl: `curl "${BASE_URL}/cv-fonts" -b cookies.txt`,
        responseExample: `{ "data": [ { "id": "tinos", "label": "Tinos", "note": "Times New Roman metrics", "css": "Tinos, \\"Times New Roman\\", Times, serif" } ] }`,
      },
      {
        method: 'POST',
        path: '/me/cvs/tailor',
        auth: 'cookie',
        summary: 'Start tailoring a CV toward one vacancy.',
        description:
          'Copies your base CV into a vacancy-bound tailored one and mints the ' +
          'short-lived credential its agent session runs on. Requires a cached match ' +
          'analysis for the job (409 otherwise) and a base CV — one is seeded from ' +
          'your stored history when you have none, and it is a 409 when there is ' +
          'nothing to seed from. Calls no LLM itself.',
        body: [{ name: 'job_slug', type: 'string', required: true, description: 'The vacancy to tailor toward.' }],
        curl: `curl -X POST "${BASE_URL}/me/cvs/tailor" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"job_slug":"senior-backend-engineer-acme-1a2b"}'`,
        responseExample: `{ "data": { "tailor_cv_id": "7d1a…", "base_cv_id": "0f2c…", "session_id": "s_9f…", "analysis": { "overall_score": 72, "verdict": "worth_applying" } } }`,
      },
      {
        method: 'POST',
        path: '/me/cvs/{id}/tailor-session',
        auth: 'cookie',
        summary: 'Mint a fresh agent session for an existing tailored CV.',
        description: 'For a tailored CV whose session was lost or never bound. 409 when the CV is not a tailored copy.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
        curl: `curl -X POST "${BASE_URL}/me/cvs/7d1a…/tailor-session" -b cookies.txt`,
        responseExample: `{ "data": { "tailor_cv_id": "7d1a…", "base_cv_id": "0f2c…", "session_id": "s_9f…" } }`,
      },
      {
        method: 'POST',
        path: '/me/cvs/base/reset-from-resume',
        auth: 'cookie',
        summary: 'Rebuild your base CV from your résumé.',
        description:
          'Replaces the base (non-tailored) document from the current résumé seed ' +
          '(experience bank + structured extract). Preserves template and typography. ' +
          'Does not rewrite tailored copies for specific jobs. 409 when there is no ' +
          'usable résumé seed.',
        curl: `curl -X POST "${BASE_URL}/me/cvs/base/reset-from-resume" -b cookies.txt`,
        responseExample: `{ "data": { "id": "0f2c…", "title": "My CV", "template_id": "classic-ats", "document": { … } } }`,
      },
      {
        method: 'POST',
        path: '/me/cvs/{id}/reset-from-resume',
        auth: 'cookie',
        summary: 'Rebuild this tailored CV from your résumé.',
        description:
          'Replaces the tailored document\'s content from the current résumé seed ' +
          '(experience bank + structured extract) and refreshes your base CV from the ' +
          'same seed. Keeps the same tailored id and agent session; preserves template ' +
          'and typography on each row. 409 when the CV is not tailored or there is no ' +
          'usable résumé seed. Upload alone does not do this — this is the explicit apply.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
        curl: `curl -X POST "${BASE_URL}/me/cvs/7d1a…/reset-from-resume" -b cookies.txt`,
        responseExample: `{ "data": { "id": "7d1a…", "title": "Tailored for …", "template_id": "classic-ats", "document": { … } } }`,
      },
      {
        method: 'GET',
        path: '/me/cvs/{id}/tailor-context',
        auth: 'cookie-or-key',
        summary: 'The match analysis a tailored CV should reframe toward.',
        description:
          'The split that keeps tailoring honest: `missing_have` are requirements ' +
          'your history already covers but the CV buries — reframe those — and ' +
          '`missing_gap` are the ones it does not, which an agent must ask about ' +
          'rather than invent. Served from cache; calls no LLM. 409 when the CV is ' +
          'not a tailored copy or has no analysis.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The tailored CV id.' }],
        curl: `curl "${BASE_URL}/me/cvs/7d1a…/tailor-context" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{
  "data": {
    "job": { "slug": "senior-backend-engineer-acme-1a2b", "title": "Senior Backend Engineer", "company": "Acme" },
    "verdict": "worth_applying",
    "overall_score": 72,
    "recommendation": "Lead with the payments migration …",
    "missing_have": [ { "requirement": "Kafka at scale", "evidence": "Ran the event bus at …" } ],
    "missing_gap": [ { "requirement": "Kubernetes operators" } ],
    "strengths": [ "Go", "payments" ],
    "gaps": [ "k8s operators" ]
  }
}`,
      },
      {
        method: 'PUT',
        path: '/me/cvs/{id}/session',
        auth: 'cookie-or-key',
        summary: 'Bind an agent session to a CV.',
        pathParams: [{ name: 'id', type: 'string (uuid)', required: true, description: 'The CV id.' }],
        body: [{ name: 'session_id', type: 'string', required: true, description: 'The agent session to bind.' }],
        curl: `curl -X PUT "${BASE_URL}/me/cvs/7d1a…/session" -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"session_id":"s_9f…"}'`,
        responseExample: `{ "data": { "id": "7d1a…", "agent_session_id": "s_9f…" } }`,
      },
    ],
  },
  {
    title: 'Experience bank',
    intro:
      'The durable record a CV is built from: employments, and the evidence atoms ' +
      'under them. Every atom carries who asserted it — you, or a model that ' +
      'inferred it — and only your own assertions may be written into a CV. ' +
      'Reading accepts an API key; editing is session-only.',
    endpoints: [
      {
        method: 'GET',
        path: '/me/experience',
        auth: 'cookie-or-key',
        summary: 'Your employments and their evidence atoms.',
        curl: `curl "${BASE_URL}/me/experience" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "employments": [ { "id": 12, "company": "Acme", "title": "Senior Backend Engineer", "started_at": "2023-02-01", "ended_at": null, "atoms": [ { "id": 88, "text": "Cut p99 checkout latency 40%", "source": "cv_import" } ] } ] } }`,
      },
      {
        method: 'PUT',
        path: '/me/experience/employments/{id}',
        auth: 'cookie',
        summary: 'Edit one employment.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The employment id.' }],
        curl: `curl -X PUT "${BASE_URL}/me/experience/employments/12" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"company":"Acme","title":"Staff Engineer"}'`,
        responseExample: `{ "data": { "id": 12, "company": "Acme", "title": "Staff Engineer" } }`,
      },
      {
        method: 'DELETE',
        path: '/me/experience/employments/{id}',
        auth: 'cookie',
        summary: 'Remove an employment and its atoms.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The employment id.' }],
        curl: `curl -X DELETE "${BASE_URL}/me/experience/employments/12" -b cookies.txt`,
      },
      {
        method: 'PUT',
        path: '/me/experience/atoms/{id}',
        auth: 'cookie',
        summary: 'Edit one evidence atom.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The atom id.' }],
        curl: `curl -X PUT "${BASE_URL}/me/experience/atoms/88" -b cookies.txt \\
  -H 'Content-Type: application/json' -d '{"text":"Cut p99 checkout latency 40% (12k rps)"}'`,
        responseExample: `{ "data": { "id": 88, "text": "Cut p99 checkout latency 40% (12k rps)", "source": "manual" } }`,
      },
      {
        method: 'DELETE',
        path: '/me/experience/atoms/{id}',
        auth: 'cookie',
        summary: 'Remove an evidence atom.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The atom id.' }],
        curl: `curl -X DELETE "${BASE_URL}/me/experience/atoms/88" -b cookies.txt`,
      },
    ],
  },
  {
    title: 'Application mail',
    intro:
      'Your job mail, and the link between a message and the application it ' +
      'belongs to. Mail arrives three ways: a connected Gmail account, the hosted ' +
      "freehire address, or a batch your own client pushed. Only the first two are " +
      'classified by freehire — pushed mail is yours to triage, which is what makes ' +
      'that tier free. Everything here takes a full-scope API key, so a harness can ' +
      'drive the whole surface; the Gmail consent redirect is the one exception and ' +
      'stays browser-only.',
    endpoints: [
      {
        method: 'GET',
        path: '/me/inbox',
        auth: 'cookie-or-key',
        summary: 'List your mail, newest first, excluding deleted.',
        description:
          '`?body=1` returns each message\'s readable text inline and marks nothing ' +
          'read — that is the agent read path, so sweeping a backlog never zeroes ' +
          "the owner's unread count (`GET /me/emails/{id}` does mark read). Pages " +
          'carrying bodies are capped at 50. An unknown `source`, `status` or `link` ' +
          'value is a 400, never a silently empty page.',
        query: [
          { name: 'source', type: 'string', description: 'One account: `gmail`, `hosted`, or `external`.' },
          { name: 'status', type: 'string', description: 'One classified label (see the triage vocabulary below).', example: 'rejection' },
          { name: 'unclassified', type: 'boolean', description: "Only mail nothing has judged yet — the agent's work queue." },
          { name: 'unread', type: 'boolean', description: 'Only messages you have not opened.' },
          { name: 'link', type: 'string', description: 'One link state: `linked`, `suggested` (awaiting your word), or `unlinked`.' },
          { name: 'q', type: 'string', description: 'Match subject, sender, or body.' },
          { name: 'body', type: 'boolean', description: 'Include each message body; caps the page at 50.' },
          { name: 'limit', type: 'integer', description: 'Page size.' },
          { name: 'offset', type: 'integer', description: 'Page offset.' },
        ],
        curl: `curl "${BASE_URL}/me/inbox?unclassified=1&body=1&limit=50" \\
  -H "Authorization: Bearer fhk_…"`,
        responseExample: `{
  "data": [
    {
      "id": 4821,
      "source": "gmail",
      "external_id": "<CAF...@mail.gmail.com>",
      "from_addr": "no-reply@us.greenhouse-mail.io",
      "from_name": "Speechify Recruiting",
      "subject": "Thank you for applying to Speechify",
      "snippet": "We have received your application…",
      "received_at": "2026-07-24T09:12:00Z",
      "read": false,
      "status_signal": "acknowledgement",
      "link_source": "thread",
      "linked_slug": "tech-lead-web-core-speechify-a1b2"
    }
  ],
  "meta": { "total": 137, "limit": 50, "offset": 0 }
}`,
      },
      {
        method: 'GET',
        path: '/me/emails/{id}',
        auth: 'cookie-or-key',
        summary: 'One message in full. Marks it read.',
        description:
          'Use `GET /me/inbox?body=1` instead when sweeping a backlog — `read_at` ' +
          'means "a human saw this", and this endpoint sets it.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.', example: '4821' }],
        curl: `curl "${BASE_URL}/me/emails/4821" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "id": 4821, "subject": "Thank you for applying to Speechify", "body_text": "Dear …", "status_signal": "acknowledgement", "linked_slug": "tech-lead-web-core-speechify-a1b2" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails',
        auth: 'cookie-or-key',
        summary: 'Push a batch your own mail client fetched.',
        description:
          'Stored under source `external`. freehire provides no transport here and ' +
          'never classifies this mail — that is the point of the tier. `external_id` ' +
          '(the Message-ID in practice) is the dedup key: re-pushing the same id ' +
          'updates that message instead of storing a copy, so a nightly re-sync is ' +
          'safe. Content columns are refreshed; your read, deleted and triage state ' +
          'is not, so a re-sync cannot resurrect deleted mail or wipe a verdict. At ' +
          'most 100 messages per call, applied as one transaction — an oversized ' +
          'batch is refused, never truncated.',
        body: [
          { name: 'messages', type: 'array', required: true, description: 'Up to 100 messages.' },
          { name: 'messages[].external_id', type: 'string', required: true, description: 'Dedup key; the Message-ID header.', example: '<CAF...@mail.gmail.com>' },
          { name: 'messages[].thread_id', type: 'string', description: 'Provider thread id, if you have one.' },
          { name: 'messages[].from_addr', type: 'string', description: 'Sender address.' },
          { name: 'messages[].from_name', type: 'string', description: 'Sender display name — the matcher reads the company name from here.' },
          { name: 'messages[].subject', type: 'string', description: 'Subject line.' },
          { name: 'messages[].body_text', type: 'string', description: 'Plain-text body.' },
          { name: 'messages[].body_html', type: 'string', description: 'HTML body; used when there is no text part.' },
          { name: 'messages[].received_at', type: 'string (RFC3339)', description: 'When the message arrived.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/emails" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"messages":[{"external_id":"<CAF...@mail.gmail.com>","from_addr":"no-reply@us.greenhouse-mail.io","subject":"Thank you for applying","received_at":"2026-07-24T09:12:00Z"}]}'`,
        responseExample: `{ "data": { "inserted": 1, "updated": 0 } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/triage',
        auth: 'cookie-or-key',
        summary: 'Record what a message is, and optionally which application it belongs to.',
        description:
          'The vocabulary is closed: `acknowledgement`, `screening`, ' +
          '`interview_invitation`, `assessment`, `offer`, `rejection`, ' +
          '`info_request`, `incomplete_application`, `other`. Anything else is ' +
          'refused. A confident verdict on a linked message advances the ' +
          "application's stage — strictly forward, never out of a settled stage, and " +
          'a rejection never advances anything by itself.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        body: [
          { name: 'signal', type: 'string', required: true, description: 'One value from the vocabulary above.', example: 'interview_invitation' },
          { name: 'slug', type: 'string', description: 'Link the message to this application at the same time.' },
          { name: 'confidence', type: 'number', description: 'Your confidence in the verdict, 0–1. Below the stage threshold the message is classified but the card does not move.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/triage" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' \\
  -d '{"signal":"interview_invitation","slug":"tech-lead-web-core-speechify-a1b2","confidence":0.9}'`,
        responseExample: `{ "data": { "id": 4821, "subject": "Interview for …", "status_signal": "interview_invitation", "linked_slug": "tech-lead-web-core-speechify-a1b2" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/link',
        auth: 'cookie-or-key',
        summary: 'Attach a message to one of your applications.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        body: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug` to attach to.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/link" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"slug":"tech-lead-web-core-speechify-a1b2"}'`,
        responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "link_source": "manual" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/unlink',
        auth: 'cookie-or-key',
        summary: "Clear a message's application link; its classification stays.",
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/unlink" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "id": 4821, "linked_slug": "", "status_signal": "acknowledgement" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/confirm',
        auth: 'cookie-or-key',
        summary: 'Accept a suggested link.',
        description:
          'Only a deterministic match — the mail thread, or the company name in the ' +
          "sender or subject — links itself. A model's pick lands here as a " +
          'suggestion instead, and `?link=suggested` is the queue of them.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/confirm" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "link_source": "confirmed" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/reject',
        auth: 'cookie-or-key',
        summary: 'Dismiss a suggested link without attaching it.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/reject" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "id": 4821, "suggested_slug": "" } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/application',
        auth: 'cookie-or-key',
        summary: 'Record an application from a message, and link the message to it.',
        description:
          'The way out of `?link=unlinked`: mail about a job you never tracked. The ' +
          "application is dated by the mail's `received_at`, not by now — the " +
          'application demonstrably existed by the time the employer wrote.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        body: [{ name: 'slug', type: 'string', required: true, description: 'The job `public_slug` to record an application for.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/application" \\
  -H "Authorization: Bearer fhk_…" \\
  -H 'Content-Type: application/json' -d '{"slug":"tech-lead-web-core-speechify-a1b2"}'`,
        responseExample: `{ "data": { "id": 4821, "linked_slug": "tech-lead-web-core-speechify-a1b2", "applied_at": "2026-07-24T09:12:00Z" } }`,
      },
      {
        method: 'POST',
        path: '/me/inbox/read-all',
        auth: 'cookie-or-key',
        summary: 'Mark every unread message matching the filters as read.',
        query: [
          { name: 'source', type: 'string', description: 'Narrow to one account.' },
          { name: 'status', type: 'string', description: 'Narrow to one label.' },
          { name: 'link', type: 'string', description: 'Narrow to one link state.' },
          { name: 'q', type: 'string', description: 'Narrow by search.' },
        ],
        curl: `curl -X POST "${BASE_URL}/me/inbox/read-all" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "marked": 12 } }`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/delete',
        auth: 'cookie-or-key',
        summary: 'Soft-delete a message. Answers 204.',
        description: 'A later re-sync will not resurrect it — deletion is reader state, not mail-server state.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/delete" -H "Authorization: Bearer fhk_…"`,
      },
      {
        method: 'POST',
        path: '/me/emails/{id}/restore',
        auth: 'cookie-or-key',
        summary: 'Undo a soft-delete. Answers 204.',
        pathParams: [{ name: 'id', type: 'integer', required: true, description: 'The message id.' }],
        curl: `curl -X POST "${BASE_URL}/me/emails/4821/restore" -H "Authorization: Bearer fhk_…"`,
      },
      {
        method: 'GET',
        path: '/me/mailbox',
        auth: 'cookie-or-key',
        summary: 'Your hosted freehire address, and whether the feature is configured.',
        description: '`address` is null until you claim one; `available` is false when the instance runs without a mailbox domain.',
        curl: `curl "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "available": true, "address": "you@mail.freehire.me" } }`,
      },
      {
        method: 'POST',
        path: '/me/mailbox',
        auth: 'cookie-or-key',
        summary: 'Claim your hosted address.',
        curl: `curl -X POST "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "available": true, "address": "you@mail.freehire.me" } }`,
      },
      {
        method: 'DELETE',
        path: '/me/mailbox',
        auth: 'cookie-or-key',
        summary: 'Release the address; it stops receiving.',
        curl: `curl -X DELETE "${BASE_URL}/me/mailbox" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "available": true, "address": null } }`,
      },
      {
        method: 'GET',
        path: '/me/gmail',
        auth: 'cookie-or-key',
        summary: 'Gmail connection status.',
        description:
          '`available` reflects whether the instance has Gmail OAuth configured. ' +
          '`status` is `needs_reconsent` when Google revoked the grant. Connecting ' +
          'itself is a browser redirect (`/me/gmail/connect`), so it is session-only ' +
          'and not part of the key-driven surface.',
        curl: `curl "${BASE_URL}/me/gmail" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "available": true, "connected": true, "email": "you@gmail.com", "status": "ok" } }`,
      },
      {
        method: 'POST',
        path: '/me/gmail/sync',
        auth: 'cookie-or-key',
        summary: 'Start an incremental sync.',
        description:
          'Returns as soon as the sync is queued — a full backfill outlives the ' +
          'request — so poll the inbox for results rather than reading counts here.',
        curl: `curl -X POST "${BASE_URL}/me/gmail/sync" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "started": true } }`,
      },
      {
        method: 'DELETE',
        path: '/me/gmail',
        auth: 'cookie-or-key',
        summary: 'Disconnect Gmail and purge the mail it synced.',
        description:
          'Best-effort revokes the grant at Google, deletes the stored token, and ' +
          'removes the Gmail-sourced mail. Mail from the hosted address stays.',
        curl: `curl -X DELETE "${BASE_URL}/me/gmail" -H "Authorization: Bearer fhk_…"`,
        responseExample: `{ "data": { "connected": false } }`,
      },
    ],
  },
];
