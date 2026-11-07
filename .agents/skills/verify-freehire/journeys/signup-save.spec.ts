// Journey: a visitor creates an account with email + password, saves a posting from the
// feed, and finds it on their tracking board. Evidence: screenshots + the saved row in Postgres
// (check it with the psql line in features/signup-and-save.md).
import { expect as baseExpect, test } from '@playwright/test';

// The dev server compiles a route on its first visit (5-15s): wait longer than the 5s default.
const expect = baseExpect.configure({ timeout: 30_000 });

const shot = (name: string) => `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/signup-save-${name}.png`;

test('register, save a job from the feed, see it on the board', async ({ page }) => {
  test.setTimeout(120_000);
  const email = `verify+${Date.now()}@example.com`;
  await page.goto('/signin#register');
  // Before hydration the form submits natively (a GET to /signin?) and nothing registers.
  await page.waitForLoadState('networkidle');
  await expect(page.getByRole('heading', { name: 'Create your account' })).toBeVisible();
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill('verify-password-123');
  await page.getByRole('button', { name: 'Create your account' }).click();
  await expect(page).not.toHaveURL(/\/signin/);
  await page.screenshot({ path: shot('1-registered'), fullPage: false });

  // A new account may get the onboarding tour as a modal over the feed, at any moment after
  // load; Playwright skips it whenever it shows up before an action.
  const tour = page.getByRole('dialog', { name: 'Get to know freehire' });
  await page.addLocatorHandler(tour, () => tour.getByRole('button', { name: 'Skip', exact: true }).click());
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  const card = page.getByTestId('job-card').first();
  const href = await card.locator('a[href^="/jobs/"]').first().getAttribute('href');
  // A full card says "Save to board" in its footer; a compact one has a glyph named "Save to your board".
  await card.getByRole('button', { name: /^Save to (your )?board$/ }).click();
  await expect(card.getByRole('button', { name: /^Remove from (your board|saved)$/ })).toBeVisible();
  await page.screenshot({ path: shot('2-saved'), fullPage: false });

  await page.goto('/my/tracking');
  await page.waitForLoadState('networkidle');
  // The board names each card "Open <title> at <company>"; read the title the API serves.
  const job = await (await page.request.get(`/api/v1${href}`)).json();
  await expect(page.getByRole('button', { name: `Open ${job.data.title.trim()} at`, exact: false })).toBeVisible();
  await page.screenshot({ path: shot('3-board'), fullPage: false });
  console.log(`VERIFY account=${email} saved=${href}`);
});
