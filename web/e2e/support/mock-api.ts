import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';

// A stand-in for the Go API, for the specs that must render real pages without a backend.
//
// Server-side rendering asks the API from the Node process, which page.route cannot see, so
// the app under test has to be started with API_INTERNAL_URL pointing at a server like this
// one. playwright.a11y.config.ts does both. Run directly, it listens on MOCK_API_PORT.
//
// Run by `node`, not by Playwright: Node strips the types itself, which is why this file has
// no enums and imports with their extension.

export const MOCK_API_PORT = Number(process.env.MOCK_API_PORT ?? 8091);

export const JOB_SLUG = 'senior-go-engineer-acme-x7k2m9qa';
export const COMPANY_SLUG = 'acme';

const job = {
  public_slug: JOB_SLUG,
  source: 'greenhouse',
  manually_added: false,
  external_id: '4001',
  url: 'https://boards.greenhouse.io/acme/jobs/4001',
  title: 'Senior Go Engineer',
  company: 'Acme',
  company_slug: COMPANY_SLUG,
  location: 'Berlin, Germany',
  description:
    '<p>Build the services behind Acme\'s job platform.</p><h2>Requirements</h2>' +
    '<ul><li>Five years of Go</li><li>PostgreSQL</li></ul>',
  countries: ['DE'],
  regions: ['europe'],
  work_mode: 'hybrid',
  skills: ['go', 'postgresql'],
  cities: ['Berlin'],
  metros: [],
  collections: [],
  perks: [],
  engagement: [],
  posted_at: '2026-11-01T09:00:00Z',
  created_at: '2026-11-01T09:00:00Z',
  updated_at: '2026-11-01T09:00:00Z',
  last_seen_at: '2026-11-07T09:00:00Z',
  enrichment: { seniority: 'senior', category: 'backend', summary: 'Backend work in Go.' },
  enrichment_version: 1,
  view_count: 12,
  applied_count: 0,
  upvote_count: 0,
  downvote_count: 0,
  my_vote: 0,
};

const secondJob = {
  ...job,
  public_slug: 'platform-engineer-acme-p4d8c1zt',
  external_id: '4002',
  title: 'Platform Engineer',
  work_mode: 'remote',
  location: 'Remote',
  cities: [],
  skills: ['kubernetes', 'terraform'],
  enrichment: { seniority: 'middle', category: 'devops' },
};

const jobs = [job, secondJob];

const company = {
  slug: COMPANY_SLUG,
  name: 'Acme',
  collections: [],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  tagline: 'Hiring infrastructure for everyone.',
  upvote_count: 0,
  downvote_count: 0,
  my_vote: 0,
  feedback_count: 0,
  feedback_rating_avg: null,
};

const facets = {
  total: jobs.length,
  facets: {
    category: { backend: 1, devops: 1 },
    countries: { DE: 1 },
    work_mode: { hybrid: 1, remote: 1 },
    seniority: { senior: 1, middle: 1 },
  },
  stats: {},
};

const scale = {
  open_jobs: 2,
  unique_open_jobs: 2,
  companies: 1,
  sources: 1,
  ats_platforms: 1,
  telegram_channels: 0,
  computed_at: '2026-11-07T00:00:00Z',
  exact: true,
};

type Answer = { status: number; body: unknown };

const ok = (body: unknown): Answer => ({ status: 200, body });
const list = (data: unknown[]): Answer =>
  ok({ data, meta: { total: data.length, limit: 20, offset: 0 } });

/** What the API answers for `path`, or null when this stand-in does not know it. */
export function answer(path: string): Answer | null {
  if (path === '/api/v1/stats/catalog') return ok({ data: scale });
  if (path === '/api/v1/jobs/facets') return ok({ data: facets });
  if (path === '/api/v1/jobs/search' || path === '/api/v1/jobs') return list(jobs);
  if (path === '/api/v1/auth/me') return { status: 401, body: { error: 'unauthorized' } };

  const jobPath = path.match(/^\/api\/v1\/jobs\/([^/]+)(?:\/(copies|similar))?$/);
  if (jobPath) {
    const found = jobs.find((j) => j.public_slug === jobPath[1]);
    if (!found) return null;
    if (jobPath[2] === 'copies') return list([]);
    if (jobPath[2] === 'similar') return ok({ data: [] });
    return ok({ data: found });
  }

  const companyPath = path.match(/^\/api\/v1\/companies\/([^/]+)$/);
  if (companyPath?.[1] === COMPANY_SLUG) {
    return ok({ data: { company, jobs, referral_available: false } });
  }
  return null;
}

export function startMockApi(port = MOCK_API_PORT) {
  const server = createServer((req, res) => {
    const path = new URL(req.url ?? '/', 'http://mock').pathname;
    const found = answer(path);
    res.setHeader('content-type', 'application/json');
    if (!found) console.error(`mock-api: nothing for ${req.method} ${req.url}`);
    const { status, body } = found ?? { status: 404, body: { error: 'not mocked' } };
    res.statusCode = status;
    res.end(JSON.stringify(body));
  });
  server.listen(port, '127.0.0.1');
  return server;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  startMockApi();
}
