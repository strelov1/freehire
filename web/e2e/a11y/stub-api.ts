import { createServer } from 'node:http';
import { catalogScale, company, COMPANY_SLUG, facetCounts, job, JOB_SLUG, plans } from './fixtures.ts';
import { STUB_PORT } from './ports.ts';

// A loopback stand-in for the Go API, just wide enough for the pages the a11y spec
// opens. The built SvelteKit server calls it from Node (API_INTERNAL_URL), which
// page.route cannot intercept. It holds fixtures only: no database, no secrets.
//
// An unhandled path answers 404 and is logged, so a page that needs a fixture it does
// not have fails loudly instead of rendering something emptier than production.

const port = Number(process.env.A11Y_STUB_PORT ?? STUB_PORT);

const list = (data: unknown[]) => ({ data, meta: { total: data.length, limit: 20, offset: 0 } });

function answer(pathname: string): unknown | undefined {
  switch (pathname) {
    case '/api/v1/jobs/facets':
      return { data: facetCounts };
    case '/api/v1/stats/catalog':
      return { data: catalogScale };
    case '/api/v1/jobs/search':
      return list([job]);
    case '/api/v1/plans':
      return { data: plans };
    case '/api/v1/auth/oauth/providers':
      return { data: ['google', 'github'] };
    case '/api/v1/threads/count':
      return { data: { count: 0 } };
    case `/api/v1/jobs/${JOB_SLUG}`:
      return { data: job };
    case `/api/v1/jobs/${JOB_SLUG}/copies`:
      return list([]);
    case `/api/v1/jobs/${JOB_SLUG}/similar`:
      return { data: [] };
    case `/api/v1/companies/${COMPANY_SLUG}`:
      return { data: { company, jobs: [job], referral_available: false } };
    default:
      return undefined;
  }
}

createServer((req, res) => {
  const { pathname } = new URL(req.url ?? '/', 'http://stub.invalid');
  // The homepage's live feed is an SSE stream; 204 makes EventSource stop for good
  // instead of holding a connection open that would keep the page from going idle.
  if (pathname === '/api/v1/feed/recent') {
    res.writeHead(204).end();
    return;
  }
  const body = answer(pathname);
  if (body === undefined) {
    console.error(`[a11y-stub] unhandled ${req.method} ${req.url}`);
    res.writeHead(404, { 'content-type': 'application/json' }).end(JSON.stringify({ error: 'not found' }));
    return;
  }
  res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(body));
}).listen(port, '127.0.0.1', () => {
  console.log(`[a11y-stub] listening on 127.0.0.1:${port}`);
});
