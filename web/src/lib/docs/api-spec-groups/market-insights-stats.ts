import { BASE_URL, type Group } from '../api-spec-types';

export const marketInsightsStatsGroup: Group = {
  title: 'Market insights & stats',
  intro:
    'Aggregate market intelligence, all public and all unauthenticated. Every ' +
    'figure is served from a precomputed rollup rather than counted live, and ' +
    'nothing here exposes a record-level field or a user identifier — these are ' +
    'counts over the catalogue, not a way to read it.',
  endpoints: [
    {
      method: 'GET',
      path: '/stats/catalog',
      auth: 'none',
      summary: 'The headline catalogue-scale figures (open jobs, companies, sources, …).',
      description:
        'The canonical numbers behind every "how big is this" figure on the ' +
        'site (the jobs list total, the /about and /open pages). Backed by one ' +
        'snapshot `cmd/rollup-stats` republishes periodically, not a live ' +
        'count. `exact: false` means the cache was cold and the figures fell ' +
        'back to a planner estimate — treat them as approximate when it is ' +
        'false.',
      curl: `curl "${BASE_URL}/stats/catalog"`,
      responseExample: `{
  "data": {
    "open_jobs": 42130,
    "companies": 6210,
    "sources": 48,
    "ats_platforms": 22,
    "telegram_channels": 340,
    "computed_at": "2026-06-18T06:00:00Z",
    "exact": true
  }
}`,
    },
    {
      method: 'GET',
      path: '/stats/embeddings',
      auth: 'none',
      summary: 'How much of the catalogue carries a semantic embedding, and how fast the rest is going.',
      description:
        'Backs the public /semantic-search page. `eligible` and `snapshot_covered` come ' +
        'from a snapshot `cmd/rollup-stats` measures every few hours (the count ' +
        'is too slow for a request); `queued` is the embedding queue, sampled ' +
        'every 15 seconds, and `covered` is `snapshot_covered` plus how much the ' +
        'queue has drained since that snapshot — not `eligible − queued`, since the ' +
        'queue also carries closed postings waiting to be cleared. ' +
        '`rate_per_minute` counts only what left the queue over the last 10 ' +
        'minutes, so a batch of new postings arriving at once does not read as ' +
        'negative progress. Any figure not yet measured is `null`, never `0`.',
      curl: `curl "${BASE_URL}/stats/embeddings"`,
      responseExample: `{
  "data": {
    "eligible": 1903114,
    "covered": 1897020,
    "snapshot_covered": 1889512,
    "queued": 6094,
    "rate_per_minute": 570.4,
    "eta_seconds": 641,
    "snapshot_at": "2026-10-19T09:22:03Z",
    "measured_at": "2026-10-19T12:04:15Z"
  }
}`,
    },
    {
      method: 'GET',
      path: '/stats/embeddings/stream',
      auth: 'none',
      summary: 'The same document as server-sent events, one per sample.',
      description:
        'A `progress` event carrying the full document on connect and after ' +
        'every 15-second sample, with a `ping` event between to keep idle ' +
        'connections alive.',
      curl: `curl -N "${BASE_URL}/stats/embeddings/stream"`,
      responseExample: `event: progress
data: {"eligible":1903114,"covered":1897020,"snapshot_covered":1889512,"queued":6094,"rate_per_minute":570.4,"eta_seconds":641,"snapshot_at":"2026-10-19T09:22:03Z","measured_at":"2026-10-19T12:04:15Z"}`,
    },
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
      description:
        'Scope by category or by country, but not both — the rollup behind it is single-dimensional, so asking for both is a 400.',
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
      description:
        'Served from the facet rollup rather than a live Meilisearch facet count, so a page can show "what\'s inside" without paying search for it.',
      curl: `curl "${BASE_URL}/stats/facets"`,
      responseExample: `{ "data": { "countries": [ { "value": "de", "count": 18420 } ], "skills": [ { "value": "go", "count": 4210 } ], "seniority": [ … ], "work_mode": [ … ] } }`,
    },
    {
      method: 'GET',
      path: '/stats/user-growth',
      auth: 'none',
      summary: "Cumulative member growth per UTC day, plus each day's own new-signup count.",
      description: 'Aggregate-only — no user identifier is exposed.',
      curl: `curl "${BASE_URL}/stats/user-growth"`,
      responseExample: `{ "data": [ { "date": "2026-07-27", "total": 12840, "new": 37 } ] }`,
    },
    {
      method: 'GET',
      path: '/stats/engagement',
      auth: 'none',
      summary: 'Jobs saved, applied to and viewed across all users, plus CV and inbox usage.',
      description:
        '`viewed` is the only field here that is not a signed-in count: it is bot-filtered job-page opens by every visitor, signed in or not, so it is orders of magnitude larger than the rest. It is also the only one that is not all-time — `viewed_since` is the first day it covers (null before the first rollup), because the bot-filtered counter shipped without a backfill of the days before it. Every other field is all-time. Aggregate-only — no user identifier is exposed.',
      curl: `curl "${BASE_URL}/stats/engagement"`,
      responseExample: `{ "data": { "saved": 41200, "applied": 18730, "viewed": 903400, "viewed_since": "2026-09-04", "cvs_uploaded": 2140, "cvs_tailored": 860, "alert_subscribers": 970, "inboxes_connected": 410, "saved_searches": 1290 } }`,
    },
    {
      method: 'GET',
      path: '/status',
      auth: 'none',
      summary: 'Ingest-fleet health, per provider.',
      description:
        'Sanitized: which boards are healthy, cooled or failing, without error text or internal identifiers.',
      curl: `curl "${BASE_URL}/status"`,
      responseExample: `{ "data": { "providers": [ { "source": "greenhouse", "boards": 812, "failing": 14, "last_run_at": "2026-07-28T18:00:00Z" } ] } }`,
    },
  ],
};
