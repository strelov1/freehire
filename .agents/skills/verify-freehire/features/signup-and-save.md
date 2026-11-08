# Sign up and save a job

A visitor creates an account with email and password, saves a posting from the feed, and
finds it on their tracking board at `/my/tracking`.

## Sub-features

- `register` creates an account on `/signin#register` and signs it in.
- `save` toggles a card's save control to the saved state.
- `board` lists the saved posting on `/my/tracking` under "No stage".

## How to get to it (user POV)

- "Sign in" in the top bar, then the register screen; or open `/signin#register` directly.
- On a job card: the footer button "Save to board" (compact cards: a glyph named "Save to your board").
- Account menu, "Tracking"; or `/my/tracking`.

## Driving it with Playwright

Preconditions: baseline; a fresh email.

- **Register.** `goto('/signin#register')`, wait for `networkidle`. Heading `Create your account`.
  Fill `getByLabel('Email')`, `getByLabel('Password')` (8+ chars), click button `Create your account`.
  The URL leaves `/signin`.
- **Save.** `goto('/jobs')`, wait for `networkidle`. The onboarding tour (dialog
  `Get to know freehire`) can cover the page at any moment: register
  `page.addLocatorHandler` on it that clicks `Close` (`Skip` may only advance one step), rather than checking once. In the first card click button
  `/^Save to (your )?board$/`. It becomes `/^Remove from (your board|saved)$/` and ENABLED (disabled means the save request is still in flight).
- **Board.** `goto('/my/tracking')`. A button `Open <title> at <company>` is visible
  (title from `GET /api/v1/jobs/<slug>`).
- **Second read.** `psql "$DATABASE_URL" -c "select u.email, j.public_slug, uj.saved_at from user_jobs uj join users u on u.id=uj.user_id join jobs j on j.id=uj.job_id where uj.saved_at is not null order by uj.saved_at desc limit 3"`
  shows the new email and slug.
- **Proof.** From the repository root run
  `.agents/skills/verify-freehire/run-journey.sh .agents/skills/verify-freehire/journeys/signup-save.spec.ts`; screenshots
  `signup-save-1-registered.png`, `-2-saved.png`, `-3-board.png`; save the psql output beside them.

## Gotchas

- A fresh account may get the onboarding tour as a modal over `/jobs`; whether it shows
  depends on where registration landed (it marks itself seen when shown). Close it when present.

- Submitting before hydration sends a native GET (`/signin?#register`) and registers nothing.
- No mail transport: the account stays unverified. Anything gated on a verified email
  (minting API keys) cannot be driven here; report it as not verified.
- The board names cards by title and company, not by link; match the title the API serves.
