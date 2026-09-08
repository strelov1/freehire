import { execFileSync } from 'node:child_process';
import { expect, test, type BrowserContext, type Page } from '@playwright/test';

// The mentorship marketplace end to end: one person publishes a mentor profile and a
// schedule, and a DIFFERENT person books an hour of it.
//
// The whole story runs through the interface except ONE step, and that exception is the
// product's own design rather than a shortcut here: a profile reaches the public only by a
// moderator's hand, and a moderator ROLE is not self-service — nothing in the app grants
// it, because anything that did would be the hole the manual gate exists to close. So the
// test grants it with one SQL statement and does the approving itself, on the real
// moderation screen.
//
// Local, like e2e/jobs-list.spec.ts: Playwright is not wired into CI here. Point it at a
// running stack:
//   PLAYWRIGHT_BASE_URL=http://localhost:5173 pnpm --dir web test:e2e
// and set MENTORSHIP_E2E_DB to the compose service if it is not the default.

const DB_SERVICE = process.env.MENTORSHIP_E2E_DB ?? 'db';

/** Run one statement against the stack's Postgres.
 *
 *  Through `docker compose exec` rather than a pg client, so the test needs no dependency
 *  the repository does not already have, and so it speaks to whatever stack the base URL
 *  points at rather than to a connection string that could drift from it. */
function sql(statement: string): string {
  return execFileSync(
    'docker',
    ['compose', 'exec', '-T', DB_SERVICE, 'psql', '-U', 'hire', '-d', 'hire', '-tAc', statement],
    { cwd: '..', encoding: 'utf8' },
  ).trim();
}

/** A fresh account. The address is unique per run: these tests write, and a re-run must
 *  not collide with what the last one left.
 *
 *  Registered through the API rather than the form, deliberately. The session cookie lands
 *  in the browser context — `context.request` shares its cookie jar with its pages — so
 *  everything after this is a real signed-in browser. The sign-in screen is somebody
 *  else's subject, and driving it here would make this test fail for reasons that have
 *  nothing to do with mentorship. The pr-smoke job in CI mints its user the same way. */
async function register(context: BrowserContext, email: string) {
  const response = await context.request.post('/api/v1/auth/register', {
    data: { email, password: 'e2e-password-not-a-secret' },
  });
  expect(response.ok(), `register ${email}: ${response.status()}`).toBe(true);

  // Past the first-run wizard, and into the beta. The marketplace ships gated: every
  // route it owns answers 404 to an account without the flag, so a test account without
  // it would exercise the gate rather than the feature.
  sql(
    `UPDATE users SET onboarding_completed_at = now(), beta_tester = true WHERE email = '${email}'`,
  );
}


/** Open a page and wait until its Svelte handlers are actually attached.
 *
 *  Without this the suite is a coin toss: Playwright clicks within a few dozen
 *  milliseconds of `goto`, and a button clicked before hydration is a button whose
 *  `onclick` does not exist yet — the DOM node is there, the click "succeeds", and nothing
 *  happens. A person never wins that race; a test wins it most of the time. Waiting for the
 *  network to settle is the cheap signal that the client bundle has run. */
async function open(page: Page, path: string) {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
}

test('a mentor publishes a profile and somebody else books an hour of it', async ({
  browser,
}) => {
  const run = Date.now();
  const mentorEmail = `e2e-mentor-${run}@example.test`;
  const seekerEmail = `e2e-seeker-${run}@example.test`;
  const slug = `e2e-mentor-${run}`;
  // Unique per run, and not decoration: the moderation queue keeps whatever earlier
  // runs left unapproved, so a shared name matches several cards and the click becomes
  // ambiguous — a test that passes alone and fails the second time it is run.
  const displayName = `E2E Mentor ${run}`;

  // The company must exist: a profile names one by `companies.slug`, and the endpoint
  // refuses a slug no row carries. That is a catalogue fact, not something a mentor
  // creates, so it is seeded rather than clicked.
  const company = `e2e-co-${run}`;
  sql(`INSERT INTO companies (slug, name) VALUES ('${company}', 'E2E Co') ON CONFLICT DO NOTHING`);

  const mentorContext = await browser.newContext();
  const mentor = await mentorContext.newPage();

  await test.step('the mentor publishes a profile', async () => {
    await register(mentorContext, mentorEmail);
    await open(mentor, '/my/mentorship/profile');

    await mentor.getByLabel('Your name, as seekers will see it').fill(displayName);
    await mentor.getByLabel('Company slug').fill(company);
    await mentor.getByLabel('Your URL').fill(slug);
    await mentor.getByLabel('Headline').fill('Staff Engineer');
    await mentor.getByLabel('Topics, comma separated').fill('career');
    await mentor.getByLabel('Languages, comma separated').fill('en');
    await mentor.getByLabel(/Meeting link/).fill('https://meet.example.test/e2e');
    await mentor.getByRole('button', { name: 'Submit for review' }).click();

    // Pending, and therefore NOT in the directory — the spec's own scenario, asserted
    // rather than waited out.
    await expect(mentor.getByText('pending')).toBeVisible();
  });

  await test.step('a pending profile is not public', async () => {
    const anonymous = await browser.newContext();
    const visitor = await anonymous.newPage();
    const response = await visitor.goto(`/mentors/${slug}`);
    expect(response?.status()).toBe(404);
    await anonymous.close();
  });

  await test.step('the mentor publishes hours', async () => {
    await open(mentor, '/my/mentorship/schedule');
    await mentor.getByRole('button', { name: 'Edit' }).click();

    // One row per weekday, wide enough that some hour is always outside the two-hour
    // minimum notice whenever this test happens to run.
    for (let weekday = 1; weekday <= 5; weekday++) {
      await mentor.getByRole('button', { name: 'Add a row' }).click();
      const row = mentor.locator('select').nth(weekday - 1);
      await row.selectOption(String(weekday));
      await mentor.locator('input[type="time"]').nth((weekday - 1) * 2).fill('09:00');
      await mentor.locator('input[type="time"]').nth((weekday - 1) * 2 + 1).fill('21:00');
    }
    await mentor.getByRole('button', { name: 'Save the week' }).click();
    await expect(mentor.getByText('Monday 09:00–21:00')).toBeVisible();
  });

  await test.step('a moderator approves it', async () => {
    // The one step no interface performs. See the file header.
    sql(`UPDATE users SET role = 'moderator' WHERE email = '${mentorEmail}'`);

    await open(mentor, '/moderation?tab=mentors');
    const card = mentor.locator('li', { hasText: displayName });
    await card.getByRole('button', { name: 'Approve' }).click();
    await expect(card).toHaveCount(0);
  });

  const seekerContext = await browser.newContext();
  const seeker = await seekerContext.newPage();

  await test.step('a different person books an hour', async () => {
    await register(seekerContext, seekerEmail);
    await open(seeker, `/mentors/${slug}`);

    // Whichever day the calendar offers, rather than a date computed here: the offerable
    // set depends on the notice period and on what day this test runs, and a hard-coded
    // date makes the test fail on a Sunday for reasons that have nothing to do with the
    // code.
    const openDay = seeker
      .getByTestId('mentor-calendar')
      .locator('button:not([disabled])')
      .first();
    await expect(openDay).toBeVisible();
    await openDay.click();

    const slot = seeker.getByTestId('mentor-slot').first();
    await expect(slot).toBeVisible();
    await slot.click();

    await seeker.getByRole('button', { name: 'Book this session' }).click();

    // The booking lands on its own session, which carries the meeting link — the thing
    // only the two parties ever see.
    await seeker.waitForURL(/\/my\/mentorship\/sessions\/[0-9a-f-]+$/);
    await expect(seeker.getByText('https://meet.example.test/e2e')).toBeVisible();
  });

  await test.step('the mentor sees who is coming', async () => {
    await open(mentor, '/my/mentorship/bookings');
    await expect(mentor.getByText(seekerEmail)).toBeVisible();
  });

  await mentorContext.close();
  await seekerContext.close();
});
