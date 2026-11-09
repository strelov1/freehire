import { BASE_URL, type Group } from '../api-spec-types';

export const jobsGroup: Group = {
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
      description:
        'Takes no filters at all — `limit` and `offset` are the whole vocabulary. ' +
        'Anything else is ignored rather than refused, and listed in ' +
        '`meta.ignored_params`: a filter param sent here returns the entire ' +
        'catalogue, which is indistinguishable from a narrow result unless you ' +
        'check. Use `/jobs/search` to filter.',
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
        'newest first; with `q`, to relevance. A param no filter reads is ignored ' +
        'rather than refused, and listed in `meta.ignored_params` — check it, ' +
        'since a dropped filter otherwise looks like a broad result.',
      filterable: true,
      query: [
        {
          name: 'q',
          type: 'string',
          description:
            'Full-text query over `title`, `company`, `description`, and `location`. Plain words use the default engine matching: the first words are required and trailing ones are dropped when too few postings match, so word order can change the result. Operators: quotes (`"systems engineer"`) require every quoted word in any order (not a contiguous phrase); `-word` excludes; uppercase `OR` between words matches any alternative (at most 5, merged and deduplicated); `title:` at the start of an alternative matches it in the title only, e.g. `title:"product owner" -junior OR title:produktchef`. Narrow the fields of the other alternatives with `q_fields`. Exact-phrase matching is not offered, and default relevance order does not reliably rank a contiguous match above a scattered-token one.',
          example: 'golang',
        },
        {
          name: 'q_fields',
          type: 'string',
          description:
            'Comma-separated subset of `title,company,description,location` to restrict `q` to, e.g. `title`. A name outside that set drops the whole parameter (reported in `meta.ignored_params`) rather than partially applying.',
          example: 'title',
        },
        {
          name: 'hide_applied_companies',
          type: 'boolean',
          description:
            '`true` excludes every posting of employers represented in the newest 1,000 applications made by the signed-in caller. Employers represented only by older applications can remain in results. Applied inside the search, so `meta.total` and paging describe what is left. A per-person preference, not a filter: a saved search carrying it narrows nothing. Without a session or API key, or with a value that is not a boolean, the search runs unhidden and the param is reported in `meta.ignored_params`.',
          example: 'true',
        },
        {
          name: 'hide_seen',
          type: 'boolean',
          description:
            '`true` leaves out the postings the signed-in caller has already opened (the 1000 most recently viewed, saved, applied to or hidden), inside the search, so `meta.total` and paging stay accurate. A per-person preference, not a filter: a saved search ignores it. Without a session or API key it is not applied and is listed in `meta.ignored_params`.',
          example: 'true',
        },
        {
          name: 'min_skill_match',
          type: 'integer',
          description:
            'Keeps only postings whose skills your profile covers by at least this percent (1–100), across the whole ranking. Applies only with `sort=match`, to a signed-in caller with profile skills; the read stops once no later posting can qualify or at a depth bound, and then `meta.total` is a minimum with `meta.total_is_lower_bound: true`. A per-person preference: a saved search ignores it. Otherwise it is not applied and is listed in `meta.ignored_params`.',
          example: '70',
        },
        {
          name: 'sort',
          type: 'string',
          description:
            'One of `created_at`, `posted_at`, `view_count`, `salary_min`, `salary_max`. Omit for relevance/newest.',
          example: 'posted_at',
        },
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
        'Same text query and facet filters as `/jobs/search`, but each result carries the ' +
        '`description` in full (verbatim from the database) instead of the truncated ' +
        'index preview. Use `description_format` to choose how it is rendered.',
      filterable: true,
      query: [
        {
          name: 'q',
          type: 'string',
          description:
            'Full-text query over `title`, `company`, `description`, and `location`. Plain words use the default engine matching: the first words are required and trailing ones are dropped when too few postings match, so word order can change the result. Operators: quotes (`"systems engineer"`) require every quoted word in any order (not a contiguous phrase); `-word` excludes; uppercase `OR` between words matches any alternative (at most 5, merged and deduplicated); `title:` at the start of an alternative matches it in the title only, e.g. `title:"product owner" -junior OR title:produktchef`. Narrow the fields of the other alternatives with `q_fields`. Exact-phrase matching is not offered, and default relevance order does not reliably rank a contiguous match above a scattered-token one.',
          example: 'golang',
        },
        {
          name: 'q_fields',
          type: 'string',
          description:
            'Comma-separated subset of `title,company,description,location` to restrict `q` to, e.g. `title`. A name outside that set drops the whole parameter (reported in `meta.ignored_params`) rather than partially applying.',
          example: 'title',
        },
        {
          name: 'description_format',
          type: 'string',
          description:
            'One of `html` (default, verbatim), `text` (tags stripped), `markdown` (HTML converted to Markdown). Unknown values fall back to `html`.',
          example: 'markdown',
        },
        {
          name: 'sort',
          type: 'string',
          description:
            'One of `created_at`, `posted_at`, `view_count`, `salary_min`, `salary_max`. Omit for relevance/newest.',
          example: 'posted_at',
        },
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
        'Takes the same `q` and facet filter params as search, but returns the ' +
        'distribution of values instead of a page of jobs — use it to build ' +
        'filter UIs or see how a filter narrows the set. Continuous numeric ' +
        'facets are returned as `stats` (min/max), not per-value buckets. ' +
        '`former_employers` is ignored even for signed-in callers.',
      query: [
        { name: 'q', type: 'string', description: 'Same full-text query as search.', example: 'golang' },
        {
          name: '(any filter)',
          type: 'string',
          description: 'Any facet filter param narrows the counted set. `former_employers` is ignored.',
          example: 'work_mode=remote',
        },
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
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'senior-go-engineer-acme-1a2b',
        },
      ],
      curl: `curl "${BASE_URL}/jobs/senior-go-engineer-acme-1a2b"`,
      responseExample: `{ "data": { "public_slug": "senior-go-engineer-acme-1a2b", "title": "Senior Go Engineer", "closed_at": null, "...": "..." } }`,
    },
    {
      method: 'GET',
      path: '/jobs/{slug}/similar',
      auth: 'none',
      summary: 'Similar jobs within the same location and specialization.',
      description:
        'Searches the live Meilisearch index with the source job’s known location and ' +
        'specialization as mandatory filters. Uses stored-vector hybrid ranking when ' +
        'available, falling back to keyword search under the same filters. Returns an ' +
        'empty list when location or specialization is unknown or no visible matches remain.',
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'senior-go-engineer-acme-1a2b',
        },
      ],
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
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'senior-go-engineer-acme-1a2b',
        },
      ],
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
    {
      method: 'GET',
      path: '/jobs/{slug}/apply-form',
      auth: 'none',
      summary: 'The captured ATS application form for the posting.',
      description:
        'The questions a candidate will have to answer, shaped for reading. ' +
        'Only captured for a minority of postings (roughly a sixth of ' +
        'technical ATS platforms are readable at all) — no captured form is ' +
        'a `404`, distinguishable from an unknown slug (also `404`, but at ' +
        'the job lookup rather than the form lookup).',
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'senior-go-engineer-acme-1a2b',
        },
      ],
      curl: `curl "${BASE_URL}/jobs/senior-go-engineer-acme-1a2b/apply-form"`,
      responseExample: `{
  "data": {
    "provider": "greenhouse",
    "basics": ["resume", "phone", "linkedin"],
    "questions": [ { "text": "Why do you want to work here?", "required": true } ]
  }
}`,
    },
    {
      method: 'GET',
      path: '/jobs/{slug}/direct-apply',
      auth: 'none',
      summary: 'The application form of a vacancy that takes applications on freehire.',
      description:
        'Only for a posting whose `direct_apply` is true — its employer published it here ' +
        'and receives applications by email. Any other posting is a `404`. Signed in with the ' +
        'browser session, the response also carries `applied`, a `prefill` of your earlier ' +
        'answers to the same questions, and `cvs` — the CV you tailored for this vacancy and ' +
        'whether you uploaded one; an API key never sees those. `open` is false once the ' +
        'vacancy closed. `501` when this deployment cannot deliver applications.',
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'platform-engineer-acme-9f3c',
        },
      ],
      curl: `curl "${BASE_URL}/jobs/platform-engineer-acme-9f3c/direct-apply"`,
      responseExample: `{
  "data": {
    "company_name": "Acme",
    "questions": [
      { "id": "q1", "label": "Do you hold an EU work permit?", "type": "boolean", "required": true },
      { "id": "q2", "label": "Preferred stack", "type": "select", "required": false, "options": ["Go", "Rust"] }
    ],
    "open": true,
    "applied": false,
    "prefill": {},
    "cvs": { "tailored": null, "has_resume": true }
  }
}`,
    },
    {
      method: 'POST',
      path: '/jobs/{slug}/direct-apply',
      auth: 'cookie',
      summary: 'Apply on freehire: the employer receives your CV and answers by email.',
      description:
        'Needs a verified email. The employer gets the chosen CV as a PDF, the answers, the ' +
        'note and your address as Reply-To. One application per vacancy: `409` once it was ' +
        'sent (or while it is being sent), `400` naming the first unanswered or invalid ' +
        'question, `403` unverified, `410` closed, `429` past the daily limit, `502` when ' +
        'delivery failed — applying again retries it.',
      pathParams: [
        {
          name: 'slug',
          type: 'string',
          required: true,
          description: 'The job `public_slug`.',
          example: 'platform-engineer-acme-9f3c',
        },
      ],
      body: [
        {
          name: 'resume',
          type: 'boolean',
          required: false,
          description: 'Send the CV file uploaded to your profile. Exactly one of `resume` and `cv_id`.',
          example: 'true',
        },
        {
          name: 'cv_id',
          type: 'string',
          required: false,
          description:
            'The CV you tailored for THIS vacancy (`cvs.tailored.id` from the form). Any other CV is a `404`.',
          example: '6f1c0c3e-8a51-4c1e-9d55-6a4c1f2b0a01',
        },
        {
          name: 'answers',
          type: 'object',
          required: false,
          description:
            'Question id → answer; a yes/no answer is `"true"` or `"false"`, a list answer one of its options.',
          example: '{"q1":"true"}',
        },
        {
          name: 'note',
          type: 'string',
          required: false,
          description: 'Up to 2000 characters to the employer.',
          example: 'Happy to talk this week.',
        },
      ],
      curl: `curl -X POST "${BASE_URL}/jobs/platform-engineer-acme-9f3c/direct-apply" \\
  -b cookies.txt -H 'Content-Type: application/json' \\
  -d '{"cv_id":"6f1c0c3e-8a51-4c1e-9d55-6a4c1f2b0a01","answers":{"q1":"true"}}'`,
      responseExample: `{ "data": { "sent": true } }`,
    },
  ],
};
