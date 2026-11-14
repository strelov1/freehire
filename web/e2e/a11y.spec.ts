import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { COMPANY_SLUG, JOB_SLUG } from './a11y/fixtures.ts';
import { STUB_PORT } from './a11y/ports.ts';

const STUB = `http://127.0.0.1:${STUB_PORT}`;

// What a page must show to count as rendered. An error page or a 404 is a perfectly
// accessible page, so without this a missing fixture would pass the scan silently.
const PAGES = [
  { name: 'home', path: '/', content: /search/i },
  { name: 'jobs', path: '/jobs', content: /Senior Backend Engineer/ },
  { name: 'job', path: `/jobs/${JOB_SLUG}`, content: /Senior Backend Engineer/ },
  { name: 'company', path: `/companies/${COMPANY_SLUG}`, content: /Logistics software for mid-size carriers/ },
  { name: 'pricing', path: '/pricing', content: /Pro/ },
  { name: 'signin', path: '/signin', content: /Sign in/ },
];

const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

// Navigation assertions can finish while the destination still fetches its data.
// Finish the stub handlers before Playwright disposes their response context.
test.afterEach(async ({ page }) => {
  await page.waitForLoadState('networkidle');
  await page.unrouteAll({ behavior: 'wait' });
});

// nginx is not here, so the browser's own /api calls are handed to the stub.
async function forwardApi(page: Page) {
  // The header badge asks our own server, which would ask api.github.com. Pin the answer:
  // the check needs no network, and the badge's label must not depend on whether it came.
  await page.route('**/github-stars', (route) => route.fulfill({ json: { stars: 3400 } }));
  await page.route('**/api/**', (route) => {
    const { pathname, search } = new URL(route.request().url());
    return route.fetch({ url: `${STUB}${pathname}${search}` }).then((res) => route.fulfill({ response: res }));
  });
}

// A changed URL precedes the destination's mounted widgets and their API calls.
async function expectJobPage(page: Page) {
  await expect(page).toHaveURL(`/jobs/${JOB_SLUG}`);
  await expect(page.getByRole('tablist', { name: 'Job details' })).toBeVisible();
  await page.waitForLoadState('networkidle');
}

for (const { name, path, content } of PAGES) {
  test(`${name} (${path}) has no serious or critical accessibility violations`, async ({ page }) => {
    await forwardApi(page);
    const response = await page.goto(path);
    expect(response?.status(), `${path} did not render (is a stub fixture missing?)`).toBe(200);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('main').first()).toBeVisible();
    await expect(page.locator('main').first()).toContainText(content);

    const results = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze();
    const blocking = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');

    const report = blocking
      .map(
        (v) =>
          `${v.id} (${v.impact}): ${v.help}\n  ${v.helpUrl}\n` +
          v.nodes.map((n) => `  - ${n.target.join(' ')}`).join('\n'),
      )
      .join('\n\n');
    expect(report, `${blocking.length} serious/critical rule(s) violated on ${path}`).toBe('');
  });
}

for (const width of [320, 393]) {
  test(`mobile job actions fit at ${width}px with a 44px Save target`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await forwardApi(page);
    await page.goto(`/jobs/${JOB_SLUG}`);

    const save = page.getByRole('button', { name: 'Save to board', exact: true });
    await expect(save).toBeVisible();
    const target = await save.boundingBox();
    expect(target?.width).toBeGreaterThanOrEqual(44);
    expect(target?.height).toBeGreaterThanOrEqual(44);

    const tabs = page.getByRole('tablist', { name: 'Job details' });
    const people = page.getByRole('link', { name: /^Find people at .+ on LinkedIn$/ });
    const draft = page.getByRole('button', { name: 'Draft referral message', exact: true });
    await expect(tabs).toBeVisible();
    await expect(people).toBeVisible();
    await expect(draft).toBeVisible();
    const tabRow = await tabs.boundingBox();
    for (const action of [people, draft]) {
      const bounds = await action.boundingBox();
      expect(bounds).not.toBeNull();
      expect(tabRow).not.toBeNull();
      if (bounds && tabRow) {
        expect(bounds.y).toBeLessThan(tabRow.y + tabRow.height);
        expect(bounds.y + bounds.height).toBeGreaterThan(tabRow.y);
      }
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
  });
}

for (const path of ['/jobs', `/companies/${COMPANY_SLUG}`]) {
  test(`job title link names only its visible title on ${path}`, async ({ page }) => {
    await forwardApi(page);
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    const card = page.getByTestId('job-card').first();
    const link = card.getByRole('heading', { name: 'Senior Backend Engineer' }).getByRole('link');
    await expect(link).toHaveText('Senior Backend Engineer');
    await expect(link).toHaveAccessibleName('Senior Backend Engineer at Acme');
    await expect(link).toHaveAccessibleDescription(/Berlin/);
    await link.focus();
    await expect(link).toBeFocused();
    const outline = await link.evaluate((el) => {
      const style = getComputedStyle(el, '::after');
      return { width: style.outlineWidth, offset: style.outlineOffset, style: style.outlineStyle };
    });
    expect(outline).toEqual({ width: '2px', offset: '-2px', style: 'solid' });
    await page.keyboard.press('Enter');
    await expectJobPage(page);
  });

  test(`job metadata opens the posting on ${path}`, async ({ page }) => {
    await forwardApi(page);
    for (const width of [393, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto(path);
      await page.waitForLoadState('networkidle');
      const card = page.getByTestId('job-card').first();
      // Hit the chip's coordinates: the stretched link intentionally covers it.
      await card.getByText('Go', { exact: true }).click({ force: true });
      await expectJobPage(page);
    }
  });
}

for (const action of ['Save to board', 'Hide this job']) {
  test(`card ${action} stays independent of posting navigation`, async ({ page }) => {
    await forwardApi(page);
    await page.goto('/jobs');
    await page.waitForLoadState('networkidle');
    const card = page.getByTestId('job-card').first();
    await card.hover();
    await card.getByRole('button', { name: action, exact: true }).click();
    await expect(page).toHaveURL(/\/signin\?returnTo=%2Fjobs&mode=login/);
  });
}

test('card Apply opens its sign-in choice without opening the posting', async ({ page }) => {
  await forwardApi(page);
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  const card = page.getByTestId('job-card').first();
  await card.hover();
  await card.getByRole('button', { name: 'Choose how to apply to Senior Backend Engineer' }).click();
  await expect(card.getByRole('button', { name: 'Continue without signing in' })).toBeVisible();
  await expect(page).toHaveURL('/jobs');
});

test('card hover discloses complete metadata on the actual link target', async ({ page }) => {
  await forwardApi(page);
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  const card = page.getByTestId('job-card').first();
  const link = card.locator('h3 a');
  await expect(link).toHaveAttribute('title', /Berlin, Germany/);
  await expect(link).toHaveAttribute('title', /Event Driven Architecture/);
  await card.getByText('Event Driven Architecture', { exact: true }).hover({ force: true });
  const targetTitle = await page.evaluate(() => {
    const hovered = document.querySelector('a:hover[title]');
    return hovered?.getAttribute('title');
  });
  expect(targetTitle).toContain('Event Driven Architecture');
});

test('overlapping country flags still open the job', async ({ page }) => {
  await forwardApi(page);
  await page.goto('/jobs');
  await page.waitForLoadState('networkidle');
  const card = page.getByTestId('job-card').first();
  await card.getByRole('img', { name: 'United States', exact: true }).click({ force: true });
  await expectJobPage(page);
});
