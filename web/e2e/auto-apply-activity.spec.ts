import { randomUUID } from 'node:crypto';
import { expect as baseExpect, test } from '@playwright/test';

const expect = baseExpect.configure({ timeout: 30_000 });

test('Activity lists the signed-in candidate’s automatic applications', async ({ page }) => {
  test.setTimeout(120_000);
  const route = '/api/v1/me/auto-apply';
  expect((await page.request.get(route)).status()).toBe(401);

  // A real isolated account starts without attempts; no application is submitted.
  const registered = await page.request.post('/api/v1/auth/register', {
    data: { email: `auto-apply-${randomUUID()}@example.test`, password: randomUUID(), timezone: 'UTC' },
  });
  expect(registered.ok()).toBe(true);
  expect((await page.request.post('/api/v1/me/onboarding/complete', { data: {} })).ok()).toBe(true);
  const listing = await page.request.get(route);
  expect(listing.ok()).toBe(true);
  expect((await listing.json()).data.attempts).toEqual([]);

  await page.goto('/my/activity');
  await page.waitForLoadState('networkidle');
  await page.screenshot({ path: `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/auto-apply-1-activity.png` });
  await page.getByRole('tab', { name: 'Auto-apply', exact: true }).click();
  await expect(page).toHaveURL(/\/my\/activity\/auto-apply$/);
  await expect(page.getByText('No automatic applications yet. Start one from a job page.')).toBeVisible();
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByText('No automatic applications yet. Start one from a job page.')).toBeVisible();
  await page.screenshot({ path: `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/auto-apply-2-empty.png` });
});
