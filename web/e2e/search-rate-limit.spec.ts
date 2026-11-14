import { expect, test, type Page } from '@playwright/test';

async function searchFor(page: Page, text: string) {
  const box = page.getByRole('combobox', { name: 'Search jobs' });
  // An SSR input is editable before hydration. The clear button proves that
  // the app received the draft before Enter commits it to the search store.
  await expect(async () => {
    await box.fill(text);
    await expect(page.getByRole('button', { name: 'Clear search', exact: true })).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 30_000 });
  await box.press('Enter');
}

// Force the gateway response at the browser boundary; the page and its initial
// postings come from the real local stack. Gateway accounting has its own HTTP suite.
test('search quota explains recovery and offers a higher plan', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  await page.route('**/api/v1/jobs/search?**', (route) =>
    route.fulfill({
      status: 429,
      contentType: 'application/json',
      headers: { 'Retry-After': '30' },
      body: JSON.stringify({
        error: 'too many requests, please try again later',
        code: 'rate_limited',
        budget: 'publicread',
        tier: 'free',
        limit: 300,
        retry_after_seconds: 30,
        upgrade_available: true,
      }),
    }),
  );
  await searchFor(page, 'engineer');
  const notice = page.getByRole('status').filter({ hasText: 'Search limit reached' });
  await expect(notice).toBeVisible({ timeout: 30_000 });
  await expect(notice).toContainText('300 requests per minute');
  await expect(notice).toContainText('Try again in 30 seconds');
  await expect(notice.getByRole('link', { name: 'Upgrade for higher search limits' })).toHaveAttribute(
    'href',
    '/pricing',
  );
  await expect(notice.getByRole('button', { name: 'Retry search' })).toBeVisible();
  await page.screenshot({ path: `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/search-quota-desktop.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(notice.getByRole('link', { name: 'Upgrade for higher search limits' })).toBeVisible();
  await page.screenshot({ path: `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/search-quota-mobile.png` });

  await page.unroute('**/api/v1/jobs/search?**');
  await page.route('**/api/v1/jobs/search?**', (route) =>
    route.fulfill({
      status: 429,
      contentType: 'text/html',
      body: '<h1>Too Many Requests</h1>',
    }),
  );
  await notice.getByRole('button', { name: 'Retry search' }).click();
  const throttled = page.getByRole('status').filter({ hasText: 'Too many search requests' });
  await expect(throttled).toBeVisible();
  await expect(throttled.getByRole('link')).toHaveCount(0);
});

test('a facets-only quota refusal offers an upgrade while keeping loaded jobs', async ({ page }) => {
  test.setTimeout(120_000);
  await page.route('**/api/v1/jobs/facets?**', (route) =>
    route.fulfill({
      status: 429,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'rate_limited',
        budget: 'publicread',
        tier: 'anonymous',
        limit: 300,
        retry_after_seconds: 30,
        upgrade_available: true,
      }),
    }),
  );
  await page.goto('/jobs?q=engineer');
  await page.waitForLoadState('networkidle');
  const notice = page.getByRole('status').filter({ hasText: 'Search limit reached' });
  await expect(notice).toBeVisible({ timeout: 30_000 });
  await expect(notice.getByRole('link', { name: 'Upgrade for higher search limits' })).toBeVisible();
  await expect(page.getByTestId('job-card').first()).toBeVisible();
});

test('one retry clears simultaneous search and facet quota refusals', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  let blocked = true;
  const failed = { search: 0, facets: 0 };
  await page.route(/\/api\/v1\/jobs\/(search|facets)\?/, async (route) => {
    if (!blocked) return route.continue();
    const kind = new URL(route.request().url()).pathname.endsWith('/facets') ? 'facets' : 'search';
    failed[kind]++;
    await route.fulfill({
      status: 429,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'rate_limited',
        budget: 'publicread',
        tier: 'free',
        limit: 300,
        retry_after_seconds: 1,
        upgrade_available: true,
      }),
    });
  });
  await searchFor(page, 'engineer');
  await expect.poll(() => failed.search, { timeout: 30_000 }).toBeGreaterThan(0);
  await expect.poll(() => failed.facets, { timeout: 30_000 }).toBeGreaterThan(0);
  const notice = page.getByRole('status').filter({ hasText: 'Search limit reached' });
  await expect(notice).toBeVisible();
  blocked = false;
  await notice.getByRole('button', { name: 'Retry search' }).click();
  await expect(page.getByTestId('job-card').first()).toBeVisible({ timeout: 30_000 });
  await expect(notice).toHaveCount(0, { timeout: 10_000 });
});
