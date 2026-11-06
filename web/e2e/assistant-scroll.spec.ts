import { createServer, type Server } from 'node:http';
import { expect, test, type Page } from '@playwright/test';

// The assistant's message list must scroll by itself: wheel and touch over it move the
// list, never the page behind it.
//
// Local, like the other specs here. Server-side rendering asks the API who is signed in
// before any /my page renders, and that call never passes through the browser, so the
// spec runs a one-route stand-in for it and the dev server must point at it:
//   API_INTERNAL_URL=http://127.0.0.1:18090 pnpm exec vite --port 5199
//   PLAYWRIGHT_BASE_URL=http://localhost:5199 pnpm exec playwright test assistant-scroll
// Everything the browser itself fetches is mocked with page.route.

const SSR_API_PORT = 18090;
const SESSION = { id: 's1', preset: 'chat', label: 'Long chat' };

const USER = {
  id: 1,
  email: 'e2e@example.com',
  role: 'user',
  beta_tester: true,
  email_verified: true,
  has_password: true,
  created_at: '2026-01-01T00:00:00Z',
  timezone: 'UTC',
  language: 'en',
  onboarding_completed_at: '2026-01-01T00:00:00Z',
};

let ssrApi: Server;
test.beforeAll(async () => {
  ssrApi = createServer((req, res) => {
    res.setHeader('content-type', 'application/json');
    res.end(JSON.stringify(req.url?.includes('/auth/me') ? { data: USER } : { data: null }));
  });
  // Workers share the one port: whichever binds first answers for all of them.
  await new Promise<void>((done) => {
    ssrApi.once('error', () => done());
    ssrApi.listen(SSR_API_PORT, '127.0.0.1', () => done());
  });
});
test.afterAll(() => ssrApi.close());

function transcript(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    seq: i + 1,
    role: i % 2 === 0 ? 'user' : 'assistant',
    content: { text: `Message ${i + 1}. ` + 'Lorem ipsum dolor sit amet. '.repeat(12) },
  }));
}

async function mockAssistant(page: Page) {
  await page.context().addCookies([{ name: 'hire_token', value: 'e2e', domain: 'localhost', path: '/' }]);
  // Anything the page asks beyond what this spec is about: answered with no data, so no request
  // waits on a backend that is not there. Registered first, so the routes below win.
  await page.route('**/api/v1/**', (route) => route.fulfill({ json: { data: null } }));
  await page.route('**/api/v1/me/experience', (route) =>
    route.fulfill({ json: { data: { employments: [], unplaced: [] } } }),
  );
  await page.route('**/api/v1/assistant/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/sessions')
      ? [SESSION]
      : { session: SESSION, messages: transcript(60) };
    await route.fulfill({ json: { data } });
  });
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ json: { data: USER } }));
}

const list = (page: Page) => page.getByRole('log');
const scroller = (page: Page) => list(page).locator('xpath=..');
async function scrollerBox(page: Page) {
  const box = await scroller(page).boundingBox();
  if (!box) throw new Error('the message list is not on screen');
  return box;
}
const pageScrollY = (page: Page) => page.evaluate(() => window.scrollY);
const state = (page: Page) =>
  scroller(page).evaluate((el) => ({
    top: el.scrollTop,
    max: el.scrollHeight - el.clientHeight,
    pageY: window.scrollY,
    docOverflow: document.documentElement.scrollHeight - window.innerHeight,
  }));

// The pages these chats sit on are taller than the viewport (the account content, the
// footer), which is what gives a wheel somewhere to go once the list stops taking it.
async function makePageScrollable(page: Page) {
  await page.evaluate(() => {
    const spacer = document.createElement('div');
    spacer.style.height = '250vh';
    spacer.setAttribute('aria-hidden', 'true');
    document.body.append(spacer);
  });
}

async function open(page: Page, url: string) {
  await mockAssistant(page);
  await page.goto(url);
  await makePageScrollable(page);
  await expect(list(page)).toContainText('Message 60.', { timeout: 20_000 });
}

async function wheelOverList(page: Page, deltaY: number) {
  const box = await scrollerBox(page);
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, deltaY);
  await page.waitForTimeout(150);
}

test.describe('full-page assistant', () => {
  test('wheel scrolls the list, not the page, and stops at the edge', async ({ page }) => {
    await open(page, '/my/assistant/s1');
    const before = await state(page);
    expect(before.max, 'transcript must overflow the list').toBeGreaterThan(500);
    expect(before.docOverflow, 'the page must have room to scroll').toBeGreaterThan(100);

    // Pinned to the newest message, and the wheel keeps pushing down: nothing is left in
    // the list to take it, and the page must not either.
    await wheelOverList(page, 400);
    expect((await state(page)).pageY).toBe(0);

    await wheelOverList(page, -400);
    const up = await state(page);
    expect(up.top).toBeLessThan(before.top);
    expect(up.pageY).toBe(before.pageY);

    // Scroll to the very top, then keep pushing: the page must not take the overflow.
    await wheelOverList(page, -100000);
    await wheelOverList(page, -400);
    const edge = await state(page);
    expect(edge.top).toBe(0);
    expect(edge.pageY).toBe(before.pageY);
  });

  test('jump-to-latest pins the list back to the newest message', async ({ page }) => {
    await open(page, '/my/assistant/s1');
    await wheelOverList(page, -100000);
    expect((await state(page)).top).toBe(0);
    await page.getByRole('button', { name: /latest/i }).click();
    await expect.poll(async () => {
      const s = await state(page);
      return s.max - s.top;
    }).toBeLessThan(4);
  });
});

test.describe('full-page assistant on a phone', () => {
  test.use({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true });

  test('touch drag scrolls the list, not the page', async ({ page }) => {
    await open(page, '/my/assistant/s1');
    const before = await state(page);
    const box = await scrollerBox(page);
    const x = box.x + box.width / 2;
    const y0 = box.y + box.height / 2;
    const cdp = await page.context().newCDPSession(page);
    const touch = (type: string, y?: number) =>
      cdp.send('Input.dispatchTouchEvent', {
        type,
        touchPoints: y === undefined ? [] : [{ x, y }],
      });
    // Finger up moves the content on, but the list is already at its end: the page must
    // stay put. Then finger down reads back through the list.
    await touch('touchStart', y0);
    for (let step = 1; step <= 10; step++) {
      await touch('touchMove', y0 - step * 30);
      await page.waitForTimeout(16);
    }
    await touch('touchEnd');
    expect((await state(page)).pageY).toBe(0);

    await touch('touchStart', y0);
    for (let step = 1; step <= 10; step++) {
      await touch('touchMove', y0 + step * 30);
      await page.waitForTimeout(16);
    }
    await touch('touchEnd');
    await page.waitForTimeout(200);
    const after = await state(page);
    expect(after.top).toBeLessThan(before.top);
    expect(after.pageY).toBe(before.pageY);
  });
});

// The docked panel (and, on a narrow screen, the covering overlay) hosts the same chat
// inside a fixed, overflow-hidden box, so the list only scrolls if every ancestor passes
// its bound down.
async function openPanel(page: Page) {
  await mockAssistant(page);
  await page.goto('/my/profile/experience');
  await page.getByRole('button', { name: 'Tailor your experience' }).first().click();
  await expect(list(page)).toContainText('Message 60.', { timeout: 20_000 });
  await makePageScrollable(page);
}

async function expectListScrolls(page: Page) {
  const before = await state(page);
  expect(before.max, 'transcript must overflow the list, not the panel').toBeGreaterThan(500);
  const pageBefore = await pageScrollY(page);
  await wheelOverList(page, 400);
  expect(await pageScrollY(page)).toBe(pageBefore);
  await wheelOverList(page, -400);
  const up = await state(page);
  expect(up.top).toBeLessThan(before.top);

  await wheelOverList(page, -100000);
  await wheelOverList(page, -400);
  expect((await state(page)).top).toBe(0);
  expect(await pageScrollY(page)).toBe(pageBefore);
}

test.describe('docked assistant panel', () => {
  test('wheel scrolls the list, not the page', async ({ page }) => {
    await openPanel(page);
    await expectListScrolls(page);
  });
});

test.describe('covering assistant panel on a phone', () => {
  test.use({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true });

  test('wheel scrolls the list, not the page', async ({ page }) => {
    await openPanel(page);
    await expectListScrolls(page);
  });
});
