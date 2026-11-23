import { expect, test } from '@playwright/test';

const job = {
  public_slug: 'backend-engineer-acme',
  title: 'Backend Engineer',
  company: 'Acme',
  location: 'Remote',
  skills: ['go', 'postgresql', 'aws'],
  requirements: [{ text: 'Production experience with AWS', priority: 'required' }],
};
const resume = 'Built payment APIs in Go and cut response times by 30%.\nSkills: PostgreSQL';

test('a guest can review a resume against a selected posting without sending its text', async ({ page }) => {
  await page.route(`**/api/v1/jobs/${job.public_slug}/skill-fit`, (route) => route.fulfill({ json: { data: job } }));
  const requestsWithResume: string[] = [];
  page.on('request', (request) => {
    if (request.postData()?.includes('Built payment APIs')) requestsWithResume.push(request.url());
  });

  await page.goto(`/tools/job-fit?job=${job.public_slug}`, { waitUntil: 'domcontentloaded' });
  await expect(page.getByRole('heading', { name: job.title })).toBeVisible();
  const input = page.getByRole('textbox', { name: 'Resume text' });
  await input.fill(resume);
  await page.getByRole('button', { name: 'Review resume', exact: true }).click();
  const results = page.getByRole('region', { name: 'What to change before you apply' });
  await expect(results).toContainText('Built payment APIs in Go and cut response times by 30%.');
  await expect(results).toContainText('Move this bullet up');
  await expect(results).toContainText('If you used PostgreSQL on a project, say what you did');
  await expect(results).toContainText('Production experience with AWS');
  await expect(results).toContainText('If you’ve used AWS, add a real example.');
  expect(requestsWithResume).toEqual([]);

  await input.fill('Built an AWS service.');
  await expect(results).toHaveCount(0);
  await page.getByRole('button', { name: 'Change job' }).click();
  await expect(page.getByRole('searchbox', { name: 'Find a job' })).toBeFocused();
});
