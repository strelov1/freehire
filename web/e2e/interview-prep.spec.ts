import { expect, test } from '@playwright/test';

const job = {
  public_slug: 'backend-engineer-acme',
  title: 'Backend Engineer',
  company: 'Acme',
  location: 'Remote',
};

test('a guest can choose a posting and open its practice sheet', async ({ page }) => {
  await page.route('**/api/v1/jobs/search?*', (route) => route.fulfill({ json: { data: [job], meta: { total: 1 } } }));
  await page.route(`**/api/v1/jobs/${job.public_slug}/interview-prep`, (route) =>
    route.fulfill({
      json: {
        data: {
          job,
          questions: [{ prompt: 'How have you operated Kubernetes in production?', topic: 'Kubernetes' }],
        },
      },
    }),
  );

  await page.goto('/tools', { waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('link', { name: /roast my cv/i })).toHaveAttribute('href', '/roast');
  await page.getByRole('link', { name: /interview prep/i }).click();
  await page.getByRole('searchbox', { name: 'Find a job' }).fill('Backend Engineer');
  await page.getByRole('button', { name: /Backend Engineer.*Acme/i }).click();
  await expect(page.getByRole('heading', { name: 'Practice questions' })).toBeFocused();
  await expect(page.getByText('How have you operated Kubernetes in production?')).toBeVisible();
  const signIn = page.getByRole('link', { name: 'Sign in to find examples' });
  const destination = new URL((await signIn.getAttribute('href')) ?? '', 'https://freehire.test');
  expect(destination.searchParams.get('returnTo')).toBe(`/tools/interview-prep?job=${job.public_slug}`);

  await page.getByRole('button', { name: 'Change job' }).click();
  await expect(page.getByRole('heading', { name: 'Practice questions' })).toHaveCount(0);
  await expect(page.getByRole('searchbox', { name: 'Find a job' })).toBeFocused();
});
