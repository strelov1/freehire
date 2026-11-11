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

// Known findings we are not fixing, each limited to one rule on one selector so every
// other rule still runs over those elements. (AxeBuilder.exclude would drop the whole
// subtree from every rule, and the card holds most of the page's text.)
const WAIVERS = [
  {
    rule: 'label-content-name-mismatch',
    selector: '[data-testid="job-card"] > a',
    // JobRow.svelte names the card link "Title at Company" on purpose: left alone its name
    // is every word on the card, read out in full before a screen-reader user learns which
    // job it is. The name still contains the title, which is what a voice user says.
    reason: 'job card link carries a deliberately short accessible name',
  },
];

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

for (const { name, path, content } of PAGES) {
  test(`${name} (${path}) has no serious or critical accessibility violations`, async ({ page }) => {
    await forwardApi(page);
    const response = await page.goto(path);
    expect(response?.status(), `${path} did not render (is a stub fixture missing?)`).toBe(200);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('main').first()).toBeVisible();
    await expect(page.locator('main').first()).toContainText(content);

    const results = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze();
    const blocking = [];
    for (const v of results.violations) {
      if (v.impact !== 'serious' && v.impact !== 'critical') continue;
      const waiver = WAIVERS.find((w) => w.rule === v.id);
      const nodes = [];
      for (const node of v.nodes) {
        const [target] = node.target;
        const waived =
          waiver !== undefined &&
          node.target.length === 1 &&
          (await page.evaluate(
            ([t, sel]) => document.querySelector(t as string)?.matches(sel as string) ?? false,
            [target, waiver.selector],
          ));
        if (!waived) nodes.push(node);
      }
      if (nodes.length > 0) blocking.push({ ...v, nodes });
    }

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
