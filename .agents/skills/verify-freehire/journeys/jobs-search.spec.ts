// Journey: a visitor opens the job feed, searches by keyword, and opens a posting.
// Needs the seeded board (stack.sh up). Evidence: three screenshots in $VERIFY_EVIDENCE.
import { expect as baseExpect, test } from '@playwright/test';

// The dev server compiles a route on its first visit (5-15s): wait longer than the 5s default.
const expect = baseExpect.configure({ timeout: 30_000 });

const shot = (name: string) => `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/jobs-search-${name}.png`;

test('feed lists postings, keyword search narrows it, a card opens the job page', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto('/jobs');
  const cards = page.getByTestId('job-card');
  await expect(cards.first()).toBeVisible();
  const before = await cards.count();
  await page.screenshot({ path: shot('1-feed'), fullPage: false });

  // SSR paints the box before Svelte hydrates it; text typed earlier is wiped on hydration.
  await page.waitForLoadState('networkidle');
  const box = page.getByRole('combobox', { name: 'Search jobs' });
  await box.fill('engineer');
  await expect(box).toHaveValue('engineer');
  await box.press('Enter');
  await expect(page).toHaveURL(/[?&]q=engineer/);
  await expect(cards.first()).toBeVisible();
  expect(await cards.count()).toBeLessThanOrEqual(before);
  await page.screenshot({ path: shot('2-search'), fullPage: false });

  const link = cards.first().locator('a[href^="/jobs/"]').first();
  const href = await link.getAttribute('href');
  // The feed may open postings in a new tab (target=_blank); follow whichever page gets it.
  const opensTab = (await link.getAttribute('target')) === '_blank';
  const [jobPage] = await Promise.all([
    opensTab ? page.context().waitForEvent('page') : Promise.resolve(page),
    link.click(),
  ]);
  await expect(jobPage).toHaveURL(new RegExp(`${href}$`));
  await expect(jobPage.getByRole('heading', { level: 1 })).toBeVisible();
  await jobPage.screenshot({ path: shot('3-job-page'), fullPage: false });
});
