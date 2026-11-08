import { createServer, type Server } from 'node:http';
import { expect, test, type Page } from '@playwright/test';

// The company watchlist: the /my/watchlist page (list, remove, bulk import) and the Watch
// toggle on a company page.
//
// Local, like the other specs here. Server-side rendering asks the API who is signed in
// (and, for the company page, for the company and its jobs) before anything renders, and
// those calls never pass through the browser, so the spec runs a small stand-in for them
// and the dev server must point at it:
//   API_INTERNAL_URL=http://127.0.0.1:18091 pnpm exec vite --port 5199
//   PLAYWRIGHT_BASE_URL=http://localhost:5199 pnpm exec playwright test watchlist
// Everything the browser itself fetches is mocked with page.route.

const SSR_API_PORT = 18091;

const USER = {
  id: 1,
  email: 'e2e@example.com',
  role: 'user',
  beta_tester: true,
  email_verified: true,
  has_password: true,
  created_at: '2026-01-01T00:00:00Z',
  timezone: 'UTC',
  language: 'en',
  onboarding_completed_at: '2026-01-01T00:00:00Z',
};

const COMPANY = {
  slug: 'acme',
  name: 'Acme Corp',
  collections: [],
  created_at: null,
  updated_at: null,
};

test.describe.configure({ mode: 'serial' });

let ssrApi: Server;
test.beforeAll(async () => {
  ssrApi = createServer((req, res) => {
    const url = req.url ?? '';
    res.setHeader('content-type', 'application/json');
    if (url.includes('/auth/me')) {
      res.end(JSON.stringify({ data: USER }));
    } else if (url.includes('/jobs/search')) {
      res.end(JSON.stringify({ data: [], meta: { total: 0, limit: 20, offset: 0 } }));
    } else if (url.includes('/companies/acme')) {
      res.end(JSON.stringify({ data: { company: COMPANY, jobs: [], referral_available: false } }));
    } else {
      res.statusCode = 404;
      res.end(JSON.stringify({ error: 'not found' }));
    }
  });
  await new Promise<void>((done, fail) => {
    ssrApi.once('error', fail);
    ssrApi.listen(SSR_API_PORT, '127.0.0.1', done);
  });
});
test.afterAll(() => new Promise<void>((done) => ssrApi.close(() => done())));

type Watched = { slug: string; name: string; added_at: string };

const entry = (slug: string, name: string): Watched => ({
  slug,
  name,
  added_at: '2026-11-01T00:00:00Z',
});

/** A stateful stand-in for the watchlist endpoints, so a removal or an import shows up in
 *  the next list read the way it would against the real backend. */
/** Open the company page and wait for the Watch state read, which only runs once the page is
 *  hydrated: a click before that lands on a button whose handler does not exist yet. */
async function openCompany(page: Page) {
  const state = page.waitForResponse(
    (r) => r.url().endsWith('/api/v1/me/watchlist/acme') && r.request().method() === 'GET',
  );
  await page.goto('/companies/acme');
  await state;
}

async function mockWatchlist(page: Page, baseURL: string | undefined, initial: Watched[]) {
  if (!baseURL) throw new Error('baseURL is not configured');
  await page.context().addCookies([{ name: 'hire_token', value: 'e2e', url: baseURL }]);
  const list = [...initial];
  const calls = { put: [] as string[], del: [] as string[], imports: [] as string[][] };

  // Anything the page asks beyond what this spec is about: answered with no data, so no
  // request waits on a backend that is not there. Registered first, so the routes below win.
  await page.route('**/api/v1/**', (route) => route.fulfill({ json: { data: [] } }));
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ json: { data: USER } }));

  await page.route('**/api/v1/me/watchlist**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const rest = url.pathname.replace('/api/v1/me/watchlist', '').replace(/^\//, '');

    if (rest === 'import') {
      const names = (request.postDataJSON() as { names: string[] }).names;
      calls.imports.push(names);
      const added: string[] = [];
      const unmatched: string[] = [];
      for (const name of names) {
        if (name.startsWith('Nope')) unmatched.push(name);
        else {
          list.push(entry(name.toLowerCase().replace(/\s+/g, '-'), name));
          added.push(name);
        }
      }
      return route.fulfill({
        json: { data: { added: added.length, already: 0, unmatched } },
      });
    }

    if (rest !== '') {
      const slug = decodeURIComponent(rest);
      if (request.method() === 'DELETE') {
        calls.del.push(slug);
        const at = list.findIndex((c) => c.slug === slug);
        if (at >= 0) list.splice(at, 1);
        return route.fulfill({ status: 204 });
      }
      if (request.method() === 'PUT') {
        calls.put.push(slug);
        const added = entry(slug, COMPANY.name);
        list.push(added);
        return route.fulfill({ json: { data: added } });
      }
      return route.fulfill({ json: { data: { watched: list.some((c) => c.slug === slug) } } });
    }

    const q = (url.searchParams.get('q') ?? '').toLowerCase();
    const shown = list.filter((c) => c.name.toLowerCase().includes(q));
    return route.fulfill({
      json: {
        data: shown,
        meta: { total: shown.length, limit: 50, offset: 0, max: 5000 },
      },
    });
  });

  return calls;
}

test.describe('watchlist page', () => {
  test('lists the watched companies and removes one', async ({ page, baseURL }) => {
    const calls = await mockWatchlist(page, baseURL, [entry('acme', 'Acme Corp'), entry('globex', 'Globex')]);
    await page.goto('/my/watchlist');

    await expect(page.getByTestId('watchlist-count')).toHaveText('2 of 5000');
    await expect(page.getByRole('link', { name: 'Acme Corp' })).toHaveAttribute('href', '/companies/acme');
    await expect(page.getByRole('link', { name: 'Globex' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Show jobs' })).toHaveAttribute('href', '/jobs?watchlist=true');

    await page.getByRole('button', { name: 'Remove Acme Corp from watchlist' }).click();

    await expect(page.getByRole('link', { name: 'Acme Corp' })).toHaveCount(0);
    await expect(page.getByTestId('watchlist-count')).toHaveText('1 of 5000');
    expect(calls.del).toEqual(['acme']);
  });

  test('narrows the list by name', async ({ page, baseURL }) => {
    await mockWatchlist(page, baseURL, [entry('acme', 'Acme Corp'), entry('globex', 'Globex')]);
    await page.goto('/my/watchlist');
    await expect(page.getByRole('link', { name: 'Globex' })).toBeVisible();

    await page.getByRole('searchbox', { name: 'Filter by name' }).fill('glob');

    await expect(page.getByRole('link', { name: 'Acme Corp' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Globex' })).toBeVisible();
  });

  test('shows the empty state with a way forward', async ({ page, baseURL }) => {
    await mockWatchlist(page, baseURL, []);
    await page.goto('/my/watchlist');

    await expect(page.getByText('Your watchlist is empty.')).toBeVisible();
    await expect(page.getByText(/press Watch on any company page/)).toBeVisible();
  });

  test('imports a list of names and reports what did not match', async ({ page, baseURL }) => {
    const calls = await mockWatchlist(page, baseURL, []);
    await page.goto('/my/watchlist');
    await expect(page.getByText('Your watchlist is empty.')).toBeVisible();

    await page.getByText('Import many at once').click();
    await page.getByLabel('Company names, one per line').fill('Initech\nNope Inc\nHooli');
    await page.getByRole('button', { name: 'Import' }).click();

    await expect(page.getByText('Added 2, already on the list 0, not matched 1.')).toBeVisible();
    await expect(page.getByLabel('Company names, one per line')).toHaveValue('Nope Inc');
    await expect(page.getByRole('link', { name: 'Initech' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Hooli' })).toBeVisible();
    await expect(page.getByTestId('watchlist-count')).toHaveText('2 of 5000');
    expect(calls.imports).toEqual([['Initech', 'Nope Inc', 'Hooli']]);
  });
});

test.describe('company page', () => {
  test('the Watch button puts the company on the watchlist and takes it off again', async ({ page, baseURL }) => {
    const calls = await mockWatchlist(page, baseURL, []);
    await openCompany(page);

    const watch = page.getByRole('button', { name: 'Watch', exact: true });
    await expect(watch).toHaveAttribute('aria-pressed', 'false');
    await watch.click();

    const watching = page.getByRole('button', { name: 'Watching', exact: true });
    await expect(watching).toHaveAttribute('aria-pressed', 'true');
    expect(calls.put).toEqual(['acme']);

    await watching.click();
    await expect(page.getByRole('button', { name: 'Watch', exact: true })).toHaveAttribute('aria-pressed', 'false');
    expect(calls.del).toEqual(['acme']);
  });

  test('starts as Watching for a company already on the list', async ({ page, baseURL }) => {
    await mockWatchlist(page, baseURL, [entry('acme', 'Acme Corp')]);
    await openCompany(page);

    await expect(page.getByRole('button', { name: 'Watching', exact: true })).toHaveAttribute('aria-pressed', 'true');
  });

  test('says so when the watchlist is full', async ({ page, baseURL }) => {
    await mockWatchlist(page, baseURL, []);
    await page.route('**/api/v1/me/watchlist/acme', (route) =>
      route.request().method() === 'PUT'
        ? route.fulfill({
            status: 400,
            json: { error: 'Your watchlist is full (5000 companies).' },
          })
        : route.fulfill({ json: { data: { watched: false } } }),
    );
    await openCompany(page);

    await page.getByRole('button', { name: 'Watch', exact: true }).click();

    await expect(page.getByText('Your watchlist is full (5000 companies).')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Watch', exact: true })).toHaveAttribute('aria-pressed', 'false');
  });
});
