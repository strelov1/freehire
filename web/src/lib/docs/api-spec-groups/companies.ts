import { BASE_URL, type Group } from '../api-spec-types';

export const companiesGroup: Group = {
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
        {
          name: 'collections',
          type: 'string',
          description: 'Curated-collection slug (e.g. `yc`, `bigtech`). Repeatable.',
          example: 'yc',
        },
        { name: 'regions', type: 'string', description: 'Region the company hires in. Repeatable.', example: 'eu' },
        { name: 'countries', type: 'string', description: 'ISO 3166-1 alpha-2 country. Repeatable.', example: 'DE' },
        {
          name: 'industries',
          type: 'string',
          description:
            'Curated industry. Matches a company through its curated industries **or** the equivalent job-derived domain. Repeatable.',
          example: 'fintech',
        },
        {
          name: 'domains',
          type: 'string',
          description:
            'Raw job-derived domain, including values no industry names (`other`, `media`, `mobility`). Repeatable.',
          example: 'fintech',
        },
        {
          name: 'company_type',
          type: 'string',
          description: 'Company type (e.g. `product`, `outstaff`). Repeatable.',
          example: 'product',
        },
        {
          name: 'company_size',
          type: 'string',
          description: 'Size bucket (e.g. `51-200`). Repeatable.',
          example: '51-200',
        },
        {
          name: 'remote_regions',
          type: 'string',
          description: 'Job-derived remote-hiring region. Repeatable.',
          example: 'eu',
        },
        { name: 'yc_batch', type: 'string', description: 'YC batch (e.g. `W21`). Repeatable.', example: 'W21' },
        { name: 'yc_status', type: 'string', description: 'YC company status. Repeatable.', example: 'active' },
        { name: 'yc_stage', type: 'string', description: 'YC funding stage. Repeatable.', example: 'series-a' },
        {
          name: 'yc_flags',
          type: 'string',
          description: 'Curated YC highlight flag. Repeatable.',
          example: 'hiring',
        },
        { name: 'maturity', type: 'string', description: 'Company stage/maturity. Repeatable.', example: 'growth' },
        {
          name: 'subindustries',
          type: 'string',
          description: 'YC subindustry leaf (see /companies/subindustries). Repeatable.',
          example: 'payments',
        },
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
      path: '/companies/{slug}/closed-jobs',
      auth: 'none',
      summary: 'A company’s closed postings — the company page’s Closed tab.',
      description:
        'A page of the postings the company has closed, most recently closed first. Closed ' +
        'postings are not in search, so this is the only list of them. There is no total: ' +
        '`meta.has_more` says whether another page follows. Paging reaches the first 2000 ' +
        'rows (`offset + limit` ≤ 2000); a deeper page is a `400`. An unknown slug is a ' +
        '`404`; a slug retired by a company merge redirects `301` to the canonical one.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      query: [
        { name: 'limit', type: 'integer', description: 'Page size (max 100).', example: '20' },
        {
          name: 'offset',
          type: 'integer',
          description: 'Rows to skip; `offset + limit` may not exceed 2000.',
          example: '0',
        },
      ],
      curl: `curl "${BASE_URL}/companies/acme/closed-jobs"`,
      responseExample: `{
  "data": [{ "public_slug": "backend-engineer-acme-1a2b3c4d", "title": "Backend Engineer", "closed_at": "2026-10-01T09:00:00Z" }],
  "meta": { "limit": 20, "offset": 0, "has_more": true }
}`,
    },
    {
      method: 'GET',
      path: '/companies/{slug}/hiring',
      auth: 'none',
      summary: 'How one company hires over time — the company page’s Hiring tab.',
      description:
        'The company’s postings added vs. removed per period (dense, missing periods ' +
        'are 0, same window rules as `/stats/jobs-activity`), its open postings now and ' +
        '30 days ago, and the median days its postings stay open before an ' +
        'employer-side closure over the last 180 days, and the page views its postings ' +
        'drew over the last 30 days and the 30 before (`views`, `views_prev`: one visitor ' +
        'counted once per posting per day, crawlers excluded). `lifetime` is omitted below 5 ' +
        'such closures — absent, never 0. An unknown slug is a `404`; a slug retired ' +
        'by a company merge redirects `301` to the canonical one.',
      pathParams: [{ name: 'slug', type: 'string', required: true, description: 'The company slug.', example: 'acme' }],
      query: [
        {
          name: 'granularity',
          type: 'string',
          description: 'One of `day`, `week`, `month` (default `day`).',
          example: 'week',
        },
        {
          name: 'from',
          type: 'string (YYYY-MM-DD)',
          description: 'Start date (UTC). Defaults to a per-granularity window before `to`.',
          example: '2026-01-01',
        },
        {
          name: 'to',
          type: 'string (YYYY-MM-DD)',
          description: 'End date (UTC). Defaults to today.',
          example: '2026-06-30',
        },
      ],
      curl: `curl "${BASE_URL}/companies/acme/hiring?granularity=week"`,
      responseExample: `{
  "data": {
    "open": 42,
    "open_prev": 30,
    "growth": 12,
    "views": 2114,
    "views_prev": 1800,
    "series": [
      { "period": "2026-06-01", "added": 5, "removed": 2 },
      { "period": "2026-06-08", "added": 3, "removed": 4 }
    ],
    "lifetime": { "closed": 37, "median_days": 21.5 }
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
};
