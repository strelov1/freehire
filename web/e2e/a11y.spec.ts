import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { COMPANY_SLUG, JOB_SLUG, answer } from './support/mock-api.ts';

// The public pages must pass axe at WCAG 2.1 A and AA for the violations that stop somebody
// using the page: impact `serious` and `critical`. `moderate` and `minor` findings are
// reported by axe but do not fail here — the gate is only worth keeping if it stays green on
// main, and those two tiers hold advice more often than defects.
//
// Runs under playwright.a11y.config.ts (`pnpm build && pnpm test:a11y`), which starts the
// production build against e2e/support/mock-api.ts. Server-side rendering reads that stand-in
// directly; the browser's own /api calls are answered from the same fixtures below.
//
// A rule that cannot be fixed without design work is switched off for the pages that need it
// in `disabledRules`, with the reason and the issue that tracks it.

const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];
const BLOCKING_IMPACTS = new Set(['serious', 'critical']);

// Every job card on the list and on the company page is one <a> whose accessible name is
// "Title at Company" (JobRow's `linkName`), kept short on purpose so a screen reader does not
// read the whole card before saying which job it is. WCAG 2.5.3 (Label in Name) wants the
// name to contain the card's visible text, which axe reports as label-content-name-mismatch.
// Meeting it means restructuring the card so the link wraps the title alone, which is design
// work. Tracked in https://github.com/strelov1/freehire/issues/4327.
const LABEL_IN_NAME = 'label-content-name-mismatch';

// `heading` is what proves the page rendered its content rather than an error page: axe
// passes an empty document just as happily. `hasJobCards` pages must also show a card, or the
// scan would pass over an empty list.
const PAGES: {
  name: string;
  path: string;
  heading: string;
  hasJobCards?: boolean;
  disabledRules: string[];
}[] = [
  { name: 'home', path: '/', heading: 'Every tech job', disabledRules: [] },
  {
    name: 'job list',
    path: '/jobs',
    heading: 'Tech jobs',
    hasJobCards: true,
    disabledRules: [LABEL_IN_NAME],
  },
  { name: 'job page', path: `/jobs/${JOB_SLUG}`, heading: 'Senior Go Engineer', disabledRules: [] },
  {
    name: 'company page',
    path: `/companies/${COMPANY_SLUG}`,
    heading: 'Acme',
    hasJobCards: true,
    disabledRules: [LABEL_IN_NAME],
  },
  { name: 'sign in', path: '/signin', heading: 'Create your account', disabledRules: [] },
];

// 1x1 transparent PNG: what a company logo (logo.freehire.me) resolves to offline.
const PIXEL = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
  'base64',
);

/** Keep the page off the network: the app's own /api calls get the fixtures, anything on
 *  another origin (logos, fonts, analytics) gets nothing or a pixel, so a run never depends on
 *  what a third party answers that day. */
async function isolate(page: Page, origin: string) {
  await page.route('**/*', async (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== origin) {
      if (route.request().resourceType() === 'image') {
        return route.fulfill({ contentType: 'image/png', body: PIXEL });
      }
      return route.abort();
    }
    if (!url.pathname.startsWith('/api/')) return route.continue();
    const found = answer(url.pathname);
    return route.fulfill({
      status: found?.status ?? 404,
      json: found?.body ?? { error: 'not mocked' },
    });
  });
}

for (const { name, path, heading, hasJobCards, disabledRules } of PAGES) {
  test(`${name} has no serious or critical accessibility violations`, async ({ page, baseURL }) => {
    if (!baseURL) throw new Error('baseURL is not configured');
    await isolate(page, new URL(baseURL).origin);

    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1 }).first()).toContainText(heading);
    if (hasJobCards) {
      await expect(page.getByTestId('job-card').first()).toBeVisible();
    }
    await page.waitForLoadState('networkidle');

    const { violations } = await new AxeBuilder({ page })
      .withTags(WCAG_TAGS)
      .disableRules(disabledRules)
      .analyze();

    const blocking = violations
      .filter((v) => v.impact && BLOCKING_IMPACTS.has(v.impact))
      .map((v) => ({
        rule: v.id,
        impact: v.impact,
        help: v.help,
        url: v.helpUrl,
        nodes: v.nodes.map((n) => n.target.join(' ')),
      }));
    expect(blocking).toEqual([]);
  });
}
