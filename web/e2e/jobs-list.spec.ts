import { expect, test } from '@playwright/test';

test('jobs list renders postings from the real pipeline', async ({ page }) => {
	await page.goto('/jobs');

	const cards = page.locator('[data-testid="job-card"]');
	await expect(cards.first()).toBeVisible();

	const firstCardLink = cards.first().locator('a[href^="/jobs/"]').first();
	await expect(firstCardLink).toHaveAttribute('href', /^\/jobs\/[^/]+$/);
});
