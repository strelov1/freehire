// Journey: a visitor opens the job feed, searches by keyword, and opens a posting.
// Needs the seeded board (stack.sh up). Evidence: three screenshots in $VERIFY_EVIDENCE.
import { expect as baseExpect, test } from '@playwright/test';

// The dev server compiles a route on its first visit (5-15s): wait longer than the 5s default.
const expect = baseExpect.configure({ timeout: 30_000 });

const shot = (name: string) => `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/jobs-search-${name}.png`;

test('feed lists postings, keyword search narrows it, a card opens the job page', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto('/jobs');
  // SSR paints the page before Svelte hydrates it: the list re-renders and text typed
  // earlier is wiped. Settle first, then count and type.
  await page.waitForLoadState('networkidle');
  const cards = page.getByTestId('job-card');
  await expect(cards.first()).toBeVisible();
  const before = await cards.count();
  expect(before).toBeGreaterThan(0);
  await page.screenshot({ path: shot('1-feed'), fullPage: false });

  const box = page.getByRole('combobox', { name: 'Search jobs' });
  await box.fill('engineer');
  await expect(box).toHaveValue('engineer');
  await box.press('Enter');
  await expect(page).toHaveURL(/[?&]q=engineer/);
  await expect(cards.first()).toBeVisible();
  expect(await cards.count()).toBeLessThanOrEqual(before);
  // The query must reach search, not just the URL: the API's total for it is below the
  // unfiltered total, and a word no posting holds empties the feed.
  const total = async (q: string) =>
    (await (await page.request.get(`/api/v1/jobs/search?limit=1${q ? `&q=${q}` : ''}`)).json()).meta.total as number;
  const [all, matched] = [await total(''), await total('engineer')];
  expect(matched).toBeGreaterThan(0);
  expect(matched).toBeLessThan(all);
  // And the cards on screen are that search's results, not the old feed left in place.
  const api = await (await page.request.get('/api/v1/jobs/search?q=engineer&limit=100')).json();
  const expected = new Set((api.data as { public_slug: string }[]).map((j) => `/jobs/${j.public_slug}`));
  const shown = await cards.locator('a[href^="/jobs/"]').evaluateAll((as) => [...new Set(as.map((a) => a.getAttribute('href')))]);
  expect(shown.length).toBeGreaterThan(0);
  for (const href of shown) expect(expected.has(href as string), `${href} is not a result for "engineer"`).toBe(true);
  await page.screenshot({ path: shot('2-search'), fullPage: false });

  await page.goto('/jobs?q=zzqxv');
  await expect(cards).toHaveCount(0);
  await page.goto('/jobs?q=engineer');
  await expect(cards.first()).toBeVisible();

  const link = cards.first().locator('a[href^="/jobs/"]').first();
  const href = await link.getAttribute('href');
  // The feed may open postings in a new tab (target=_blank); follow whichever page gets it.
  const opensTab = (await link.getAttribute('target')) === '_blank';
  const [jobPage] = await Promise.all([
    opensTab ? page.context().waitForEvent('page') : Promise.resolve(page),
    link.click(),
  ]);
  await expect(jobPage).toHaveURL(new RegExp(`${href}$`));
  const job = await (await page.request.get(`/api/v1${href}`)).json();
  await expect(jobPage.getByRole('heading', { level: 1 })).toContainText(job.data.title.trim());
  await jobPage.screenshot({ path: shot('3-job-page'), fullPage: false });
});
