import { expect, test } from '@playwright/test';

test.use({ viewport: { width: 375, height: 812 } });

test('mobile account navigation remains available without a CV', async ({ page }) => {
  test.setTimeout(120_000);
  const registration = await page.request.post('/api/v1/auth/register', {
    data: { email: `verify-mobile-${Date.now()}@example.com`, password: 'verify-password-123' },
  });
  expect(registration.ok()).toBe(true);
  const tour = page.getByRole('dialog', { name: 'Get to know freehire' });
  await page.addLocatorHandler(tour, () => tour.getByRole('button', { name: 'Close', exact: true }).click());
  await page.goto('/my/profile/cv-readiness');
  await page.waitForLoadState('networkidle');
  const navigation = page.getByRole('button', { name: /Account sections:/ });
  await expect(navigation).toBeVisible({ timeout: 30_000 });
  await navigation.click();
  const drawer = page.getByRole('dialog');
  await expect(drawer).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(drawer).not.toBeVisible();
  await navigation.click();
  await drawer.getByRole('link', { name: 'Profile', exact: true }).click();
  await expect(page).toHaveURL(/\/my\/profile$/);
  await expect(navigation).toBeVisible();
  await page.screenshot({ path: `${process.env.VERIFY_EVIDENCE ?? 'test-results'}/mobile-profile.png` });
});
