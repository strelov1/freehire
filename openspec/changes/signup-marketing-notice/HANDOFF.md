# Handoff: record what people were told about email when they sign up

Written 2026-09-08, after the email preference centre shipped (PR #2673, live on
prod the same evening). This is the other half of the same law, and it has not
been started — there is no code yet, only this note.

Read this whole file before touching anything. Everything in the "Facts already
established" section was verified against the tree on 2026-09-08; re-check
anything that smells stale rather than trusting it.

---

## Why this exists

Turning mail off is now solved: every non-essential mail carries an unsubscribe
link and RFC 8058 headers, and `/unsubscribe` opens without a session. That
satisfies CAN-SPAM and the Gmail/Yahoo bulk-sender rules.

It does **not** satisfy the collection side. GDPR Art. 7(1) requires being able
to **demonstrate** the lawful basis for marketing email — when, and against what
wording. Today the product records nothing: an account is created and starts
receiving the founder sequence and campaigns with no evidence anybody was told.

## The decision already made

The owner chose **soft opt-in with recorded evidence**, not a consent checkbox.
Do not re-open this without a reason; the trade was understood when it was made:

- **ePrivacy Art. 13(2)** permits emailing your own users about similar things
  without prior consent, *provided* an opt-out was offered when the address was
  collected **and** in every message. The second half shipped on 2026-09-08. The
  missing half is a line at signup.
- A **pre-ticked box is worthless** — CJEU, *Planet49* (C-673/17). If a box were
  used it would have to start empty, and an empty box is ticked by a few percent,
  which effectively ends the campaign and founder-sequence lists.
- So: a **notice**, not a checkbox. It costs no list size and it is what the
  regulation actually asks for in this shape.

The valuable part is **not the sentence on the page**. It is the stored evidence:
when it was shown, and *which version* of the wording. Six months from now the
copy will have changed, and without a version nobody can say what a September
signup actually agreed to. Evidence that cannot name its own text is not
evidence.

## Facts already established (verified 2026-09-08)

**Account creation funnels through one place.** `grep` for callers of
`CreateUser` outside `internal/platform/db` returns exactly three, and two of
them are in the same file:

- `internal/identity/accounts/accounts.go:269` — `Register`, the password path.
- `internal/identity/accounts/repository.go:104` — inside the OAuth
  identity-resolution transaction; creates a passwordless, born-verified account.
- `internal/identity/accounts/repository.go:152` — `QueriesRepository.CreateUser`,
  which `Register` calls.

So one stamp point can cover every web signup. **The mobile app is a separate
repository** (`freehire-mobile`) and its registrations will carry no notice until
it is changed there too — say so in the proposal rather than discovering it after.

**There is no consent column today.** `grep -rn "marketing\|consent\|opted"
migrations/*.sql` finds nothing about email consent (the hits are Gmail
re-consent, CV tracer links, and two Telegram channel names).

**The signup UI is one page**: `web/src/routes/signin/+page.svelte`, 362 lines,
four screens switched by the URL hash (`#register`, `#login`, `#forgot`,
`#reset`). The OAuth buttons live on the same page, so one line placed under the
submit button covers both the password and the provider paths — check that it is
visible on the `register` screen specifically, not only on `login`.

`/terms` and `/privacy` both exist as routes and can be linked from the notice.

## Suggested shape (not a decision — brainstorm it)

- One migration: `users.marketing_notice_at timestamptz` and
  `users.marketing_notice_version text`. Nullable, no backfill — a NULL means
  "predates this", which is the truth and is worth being able to see.
- Stamp both at account creation, from a version constant that lives beside the
  copy so the two cannot drift.
- The line under the signup button, roughly: *"We'll email you alerts for the
  searches you save, plus the occasional note about what we've built. You can
  stop any of it at any time."* — with the second half linking to
  `/my/notifications/settings`. Get the wording reviewed; it is the thing being
  recorded.
- Decide deliberately what the version string is. A date is tempting and wrong:
  it says when the text was written, not which text. A short content hash, or a
  hand-bumped `v1`, both say the right thing.

## Traps this codebase will spring on you

- **Migration number.** Take the next free one and **re-check it against
  `origin/main` immediately before opening the PR**, not when creating the file.
  `main` took `0152` out from under the email-prefs branch mid-flight; nothing
  gates a collision and the runner orders by filename.
- **`internal/platform/db/` is generated.** Edit `queries/*.sql`, run `make
  sqlc`. A `git checkout` of a generated file reverts to the *committed* version
  and silently undoes an un-regenerated change — that happened once during the
  email-prefs work.
- **New package ⇒ `internal/platform/arch/layering/blocks.go`.** The guard is
  symmetric: a package with no entry fails, and an entry with no package fails
  too. Neither half ships alone. Probably not needed here — this is likely all
  inside `identity` — but check.
- **`go test ./...` compiles no `//go:build integration` file.** Run
  `go vet -tags=integration ./...` before every push.
- **Go's test cache hides mutation checks.** A guard test re-run without
  `-count=1` can report `(cached)` and look like it passed. It bit this work
  once.
- **`knip` gates unused *exports*, types included.** An exported wire type that
  nothing imports fails CI's `dead-code` job. Declare it unexported unless a
  consumer names it.
- **A fresh worktree needs `pnpm --dir web exec svelte-kit sync`** or `pnpm
  check` reports dozens of phantom errors that have nothing to do with the change.

## The lesson worth carrying over

The email-prefs review caught a bug the test suite did not: the new preference
read coalesced a missing `notification_settings` row to `false` while the
delivery query coalesced it to `true`, so one click on a *campaign's* unsubscribe
button silently turned off somebody's saved-job reminders.

The test that should have caught it existed, seeded exactly that state, and
passed — because it only asserted the field it was changing and never looked at
the field next to it. **A test that does not check the neighbouring field proves
the button works, not that it is isolated.** Write the consent tests with that
in mind: assert what must NOT have changed.

## Still owed from the previous change

Not blocking this work, but do not lose them:

1. **nginx**: switch the access log off for the one-click path. The token never
   expires and nginx logs query strings; RFC 8058 fixes the request body, so the
   URL is the only place Gmail can carry it. **`freehire-api.conf` is not in this
   repo** — it lives only on the host, so this needs the host file first.
   `./deploy/check-drift.sh` is what reports the gap.
2. **Archive the `email-preference-center` change** — move it under
   `openspec/changes/archive/` and sync the deltas into `openspec/specs/`, so
   `openspec/specs/` stops disagreeing with what shipped.
3. **Reply to the complainant** (Juril), whose message started all of this. His
   link works now.
