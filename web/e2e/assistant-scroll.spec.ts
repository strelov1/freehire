import { createServer as createMockBackend, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { createServer as createViteServer, type ViteDevServer } from 'vite';
import { expect as baseExpect, test as base, type Page, type Route } from '@playwright/test';

// The assistant's message list must scroll by itself: wheel, trackpad and touch over it move
// the list, never the page behind it, and the edge of the list does not hand the gesture on.
//
// Self-contained: it starts its own Vite dev server pointed at a tiny in-process backend, so
// no stack needs to be running. The backend answers only what a SvelteKit `load` asks the
// server for (`/me`, which no `page.route` can see because it never crosses the browser);
// everything the browser fetches is mocked with `page.route`.
//   pnpm exec playwright test e2e/assistant-scroll.spec.ts

const ASSISTANT = '/api/v1/assistant';
const SUPPORT = '/api/v1/support';

const USER = {
  id: 1,
  email: 'e2e@example.test',
  role: 'user',
  beta_tester: false,
  email_verified: true,
  has_password: true,
  created_at: '2026-01-01T00:00:00Z',
  timezone: 'UTC',
  language: 'en',
  onboarding_completed_at: '2026-01-01T00:00:00Z',
  tier: 'pro',
  agent_connected: false,
};

type App = { origin: string };

const test = base.extend<{ baseURL: string }, { app: App }>({
  app: [
    // Playwright reads a fixture's dependencies from the destructured first argument and rejects anything else.
    // oxlint-disable-next-line no-empty-pattern
    async ({}, use) => {
      const backend: Server = createMockBackend((req, res) => {
        res.setHeader('content-type', 'application/json');
        if (req.url?.startsWith('/api/v1/auth/me')) {
          res.end(JSON.stringify({ data: USER }));
          return;
        }
        res.statusCode = 404;
        res.end(JSON.stringify({ error: 'not mocked' }));
      });
      await new Promise<void>((done) => backend.listen(0, '127.0.0.1', done));
      process.env.VITE_API_URL = `http://127.0.0.1:${(backend.address() as AddressInfo).port}`;

      const vite: ViteDevServer = await createViteServer({
        root: process.cwd(),
        server: { host: '127.0.0.1', port: 0, strictPort: false },
        logLevel: 'error',
      });
      await vite.listen();
      const origin = vite.resolvedUrls?.local[0]?.replace(/\/$/, '');
      if (!origin) throw new Error('vite did not report an address');

      // The first render of a route compiles the whole app (~1 min cold); pay it here, once,
      // rather than inside whichever test happens to ask first.
      await Promise.all(
        ['/my/assistant', '/my/notifications'].map((path) =>
          fetch(`${origin}${path}`, { headers: { cookie: 'hire_token=warmup' } }),
        ),
      );

      await use({ origin });

      await vite.close();
      await new Promise((done) => backend.close(done));
    },
    { scope: 'worker', timeout: 300_000 },
  ],
  baseURL: async ({ app }, use) => use(app.origin),
});

// One long reply, so the transcript is far taller than any viewport used here.
const paragraph = (n: number) => `Paragraph ${n}. ${'The quick brown fox jumps over the lazy dog. '.repeat(8)}`;
const LONG_REPLY = Array.from({ length: 60 }, (_, n) => paragraph(n)).join('\n\n');

// The browser compiles the client modules on first load too, so allow for a cold dev server.
test.beforeEach(() => test.setTimeout(120_000));
const expect = baseExpect.configure({ timeout: 45_000 });

const SESSION = { id: 's1', preset: 'chat', label: 'Long chat' };
const TRANSCRIPT = Array.from({ length: 30 }, (_, n) => ({
  seq: n + 1,
  role: n % 2 === 0 ? ('user' as const) : ('assistant' as const),
  content: { text: n % 2 === 0 ? `Question ${n}` : paragraph(n) },
}));

const json = (route: Route, data: unknown) =>
  route.fulfill({ contentType: 'application/json', body: JSON.stringify({ data }) });

function sse(events: object[]): string {
  return events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join('');
}

/** Everything the full-page assistant fetches. `gate`, when given, holds each turn's reply
 *  until the test releases it, so the test can scroll while a reply is still on its way. */
async function mockAssistant(page: Page, gate?: Promise<void>) {
  await page.route(`**${ASSISTANT}/sessions`, (route) =>
    route.request().method() === 'GET' ? json(route, [SESSION]) : json(route, SESSION),
  );
  await page.route(`**${ASSISTANT}/sessions/s1`, (route) =>
    json(route, { session: SESSION, messages: TRANSCRIPT }),
  );
  await page.route(`**${ASSISTANT}/sessions/s1/messages`, async (route) => {
    await gate;
    await route.fulfill({
      contentType: 'text/event-stream',
      body: sse([
        { type: 'assistant_text', text: 'Fresh reply that arrives last.' },
        { type: 'result', stop_reason: 'end_turn' },
      ]),
    });
  });
}

async function signIn(page: Page, origin: string) {
  await page.context().addCookies([{ name: 'hire_token', value: 'e2e', url: origin }]);
}

// The transcript's parent is the scroller; the transcript itself is as tall as the whole
// conversation, so its own box says nothing about what is on screen.
const LIST = '[role="log"] >> xpath=..';

/** The geometry that matters, read in the page. */
async function metrics(page: Page) {
  return page.evaluate(() => {
    const list = document.querySelector<HTMLElement>('[role="log"]')?.parentElement;
    if (!list) throw new Error('no message list');
    return {
      listTop: list.scrollTop,
      listMax: list.scrollHeight - list.clientHeight,
      overflows: list.scrollHeight > list.clientHeight,
      pageY: window.scrollY,
    };
  });
}

async function wheelOverList(page: Page, deltaY: number) {
  const box = await page.locator(LIST).boundingBox();
  if (!box) throw new Error('message list is not visible');
  await page.mouse.move(box.x + box.width / 2, box.y + Math.min(box.height / 2, 200));
  await page.mouse.wheel(0, deltaY);
}

async function send(page: Page, text: string) {
  await page.getByRole('textbox').last().fill(text);
  await page.keyboard.press('Enter');
}

/** A one-finger swipe, the way a phone delivers it; Playwright has no touch scroll of its own.
 *  Positive `deltaY` scrolls down, like a wheel: the finger travels up. */
async function swipeOverList(page: Page, deltaY: number) {
  const box = await page.locator(LIST).boundingBox();
  if (!box) throw new Error('message list is not visible');
  const x = Math.round(box.x + box.width / 2);
  const startY = Math.round(box.y + box.height / 2);
  const cdp = await page.context().newCDPSession(page);
  const touch = (type: string, y: number) =>
    cdp.send('Input.dispatchTouchEvent', {
      type,
      touchPoints: type === 'touchEnd' ? [] : [{ x, y }],
    });
  const steps = 12;
  const travel = Math.max(-box.height / 2 + 20, Math.min(box.height / 2 - 20, -deltaY));
  await touch('touchStart', startY);
  for (let i = 1; i <= steps; i++) {
    await touch('touchMove', Math.round(startY + (travel * i) / steps));
    await page.waitForTimeout(16);
  }
  await touch('touchEnd', 0);
  await cdp.detach();
}

async function openFullPage(page: Page, origin: string) {
  await signIn(page, origin);
  await mockAssistant(page);
  await page.goto('/my/assistant');
  await expect(page.getByText('Paragraph 29.').first()).toBeAttached();
  await expect.poll(async () => (await metrics(page)).overflows).toBe(true);
}

async function openSupportPanel(page: Page, origin: string) {
  await signIn(page, origin);
  await page.route(`**${SUPPORT}/sessions`, (route) => json(route, { id: 'sup1', preset: 'support', label: 'Support' }));
  await page.route(`**${SUPPORT}/sessions/sup1/messages`, (route) =>
    route.fulfill({
      contentType: 'text/event-stream',
      body: sse([
        { type: 'assistant_text', text: LONG_REPLY },
        { type: 'result', stop_reason: 'end_turn' },
      ]),
    }),
  );
  await page.goto('/my/notifications');
  // A click that lands before hydration is lost, so click until the panel reports open.
  const help = page.getByRole('button', { name: 'Help', exact: true });
  await expect(async () => {
    if ((await help.getAttribute('aria-expanded')) !== 'true') await help.click();
    await expect(help).toHaveAttribute('aria-expanded', 'true', { timeout: 1_000 });
  }).toPass();
  await page.getByRole('textbox').last().fill('Tell me everything');
  await page.keyboard.press('Enter');
  await expect(page.getByText('Paragraph 59.').first()).toBeAttached();
  await expect.poll(async () => (await metrics(page)).overflows).toBe(true);
}

/** The page must be able to scroll at all, or "the page did not move" proves nothing. */
async function expectPageScrollable(page: Page) {
  const room = await page.evaluate(
    () => document.documentElement.scrollHeight - document.documentElement.clientHeight,
  );
  expect(room, 'the page has nowhere to scroll, so a chained scroll would be invisible').toBeGreaterThan(0);
}

test.describe('full-page assistant (/my/assistant)', () => {
  test.use({ viewport: { width: 1280, height: 720 } });

  test('wheel over the list scrolls the list, not the page', async ({ page, app }) => {
    await openFullPage(page, app.origin);
    const before = await metrics(page);
    const start = before.listTop;

    await wheelOverList(page, -400);
    await wheelOverList(page, 400);
    await expect.poll(async () => (await metrics(page)).listTop).not.toBe(start);
    expect((await metrics(page)).pageY).toBe(0);
  });

  test('the end of the list does not hand the scroll to the page', async ({ page, app }) => {
    await openFullPage(page, app.origin);

    await wheelOverList(page, 100_000);
    await expect.poll(async () => {
      const m = await metrics(page);
      return Math.abs(m.listMax - m.listTop) < 2;
    }).toBe(true);
    await wheelOverList(page, 600);
    await page.waitForTimeout(300);
    expect((await metrics(page)).pageY).toBe(0);
  });
});

test.describe('full-page assistant on a phone', () => {
  test.use({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true });

  test('a swipe over the list scrolls the list, not the page', async ({ page, app }) => {
    await openFullPage(page, app.origin);
    const start = (await metrics(page)).listTop;

    await swipeOverList(page, -400);
    await expect.poll(async () => (await metrics(page)).listTop).not.toBe(start);
    expect((await metrics(page)).pageY).toBe(0);

    await swipeOverList(page, 100_000);
    await page.waitForTimeout(300);
    expect((await metrics(page)).pageY).toBe(0);
  });
});

test.describe('docked support panel', () => {
  test.use({ viewport: { width: 1440, height: 800 } });

  test('wheel over the list scrolls the list, not the page', async ({ page, app }) => {
    await openSupportPanel(page, app.origin);
    await expectPageScrollable(page);
    const start = (await metrics(page)).listTop;

    await wheelOverList(page, -400);
    await expect.poll(async () => (await metrics(page)).listTop).not.toBe(start);
    expect((await metrics(page)).pageY).toBe(0);

    // Wheel scrolling latches onto the element it started on for a moment, so a second wheel
    // at the edge only tests chaining once that has lapsed.
    await wheelOverList(page, 100_000);
    await expect.poll(async () => {
      const m = await metrics(page);
      return Math.abs(m.listMax - m.listTop) < 2;
    }).toBe(true);
    await page.waitForTimeout(1_000);
    await wheelOverList(page, 600);
    await page.waitForTimeout(500);
    expect((await metrics(page)).pageY).toBe(0);
  });
});

test.describe('covering support panel on a phone', () => {
  test.use({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true });

  test('a swipe over the list scrolls the list, and the page stays put', async ({ page, app }) => {
    await openSupportPanel(page, app.origin);
    const start = (await metrics(page)).listTop;

    await swipeOverList(page, -400);
    await expect.poll(async () => (await metrics(page)).listTop).not.toBe(start);
    expect((await metrics(page)).pageY).toBe(0);
  });
});

test.describe('following the newest message', () => {
  test.use({ viewport: { width: 1280, height: 720 } });

  test('a reply scrolls into view when the reader is at the bottom', async ({ page, app }) => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => (release = resolve));
    await signIn(page, app.origin);
    await mockAssistant(page, gate);
    await page.goto('/my/assistant');
    await expect.poll(async () => (await metrics(page)).overflows).toBe(true);

    await send(page, 'Anything new?');
    release();
    await expect(page.getByText('Fresh reply that arrives last.')).toBeInViewport();
  });

  test('a reply leaves a reader who scrolled up where they were, and the button brings them back', async ({
    page,
    app,
  }) => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => (release = resolve));
    await signIn(page, app.origin);
    await mockAssistant(page, gate);
    await page.goto('/my/assistant');
    await expect.poll(async () => (await metrics(page)).overflows).toBe(true);

    await send(page, 'Anything new?');
    await wheelOverList(page, -100_000);
    await expect.poll(async () => (await metrics(page)).listTop).toBeLessThan(5);
    release();
    await expect(page.getByText('Fresh reply that arrives last.')).toBeAttached();
    expect((await metrics(page)).listTop).toBeLessThan(5);

    await page.getByRole('button', { name: 'Jump to latest' }).click();
    await expect(page.getByText('Fresh reply that arrives last.')).toBeInViewport();
  });
});
