import type { Company, FacetCounts, Job, PlansMatrix, CatalogScale } from '../../src/lib/types.ts';

// Invented data only. Typed against the app's own wire types so a field the pages
// start requiring shows up here as a type error rather than a half-rendered page.

export const JOB_SLUG = 'senior-backend-engineer-acme-1a2b3c4d';
export const COMPANY_SLUG = 'acme';

const NOW = '2026-11-01T09:00:00Z';

export const job: Job = {
  public_slug: JOB_SLUG,
  source: 'greenhouse',
  manually_added: false,
  external_id: '1001',
  url: 'https://boards.example.com/acme/jobs/1001',
  title: 'Senior Backend Engineer',
  company: 'Acme',
  company_slug: COMPANY_SLUG,
  location: 'Berlin, Germany',
  description:
    '<p>Acme builds logistics software. You will design and run the services behind it.</p>' +
    '<h2>Requirements</h2><ul><li>5+ years of Go</li><li>PostgreSQL in production</li></ul>',
  countries: ['DE'],
  regions: ['eu'],
  work_mode: 'hybrid',
  skills: ['go', 'postgresql'],
  cities: ['Berlin'],
  metros: [],
  collections: [],
  perks: [],
  engagement: [],
  posted_at: NOW,
  created_at: NOW,
  updated_at: NOW,
  last_seen_at: NOW,
  enrichment: {
    summary: 'Backend role on a logistics platform.',
    work_mode: 'hybrid',
    employment_type: 'full_time',
    seniority: 'senior',
    category: 'backend',
    skills: ['go', 'postgresql'],
    salary_min: 90000,
    salary_max: 120000,
    salary_currency: 'EUR',
    salary_period: 'year',
  },
  enrichment_version: 1,
  view_count: 120,
  applied_count: 4,
  upvote_count: 0,
  downvote_count: 0,
  my_vote: 0,
};

export const company: Company = {
  slug: COMPANY_SLUG,
  name: 'Acme',
  collections: [],
  created_at: NOW,
  updated_at: NOW,
  industries: ['logistics'],
  year_founded: 2012,
  employee_count: 250,
  hq_country: 'DE',
  tagline: 'Logistics software for mid-size carriers.',
  upvote_count: 0,
  downvote_count: 0,
  my_vote: 0,
  feedback_count: 0,
  feedback_rating_avg: null,
};

export const facetCounts: FacetCounts = {
  total: 1,
  facets: {
    category: { backend: 1 },
    countries: { DE: 1 },
  },
  stats: {},
};

export const catalogScale: CatalogScale = {
  open_jobs: 1,
  unique_open_jobs: 1,
  companies: 1,
  sources: 1,
  ats_platforms: 1,
  telegram_channels: 0,
  computed_at: NOW,
  exact: true,
};

export const plans: PlansMatrix = {
  features: [
    {
      feature: 'fit_analysis',
      free: { limit: 0, unlimited: false },
      pro: { limit: 10, unlimited: false },
      ultra: { limit: 0, unlimited: true },
    },
  ],
  prices: [
    { id: 'price_pro_month', amount_cents: 1200, currency: 'USD', interval: 'month', default: true, tier: 'pro' },
    { id: 'price_ultra_month', amount_cents: 2900, currency: 'USD', interval: 'month', default: true, tier: 'ultra' },
  ],
  enforced: [],
};
