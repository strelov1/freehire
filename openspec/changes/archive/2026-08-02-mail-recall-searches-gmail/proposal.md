## Why

The mailbox sweep shipped in #1514 asks the wrong store, and production says so plainly.

It gathers unattached mail from the copy we already synced and hands the model 40 of it.
Candidates in that window run **96 at minimum, 158 at the median**, and **263 of 263
applications exceed the cap** — every one, by 2.4× at best. The order is deterministic, so
pressing again returns the same 40 and the rest are unreachable by any number of presses.
Two real presses returned `scanned: 40, suggested: []`.

The 40 are also chosen for no reason: oldest first, which says nothing about relevance. The
employer's name cannot rescue the ordering either — matched against sender name and subject
it yields a **median of 0**, present at all for only 110 of 263 applications.

And part of the mail is not in the store to begin with. The sync fetches only ATS domains
and twelve phrases: over 120 days it took **431** messages while the mailbox holds **3297**
and a hiring-shaped query finds **1151**. Among the **739** it never fetched are an
acknowledgement, three interview invitations and four live recruiter threads.

Asking Gmail per employer inverts the result. Across 15 applications with ~100 candidates
each and **zero** linked messages today, one Gmail query found mail for **14 of 15** —
because Gmail searches the body and our SQL net could not.

## What Changes

- The sweep **searches Gmail** for the employer inside the application's window, instead of
  reading the `emails` table. Typical result: 0–9 candidates rather than 158.
- The cap, the ordering and the ranking problem are **removed**, not tuned — there is
  nothing left to rank.
- The search is **gated to job-shaped mail**, so scoping by a company name cannot pull
  personal correspondence. The gate is employer AND (hiring words OR `filename:ics` OR the
  role title); both halves were measured, and the narrower first version was rejected.
- A proposal is a **Gmail message, not a stored row**. It lives on screen; pressing Link
  imports it and then links it. **Nothing unconfirmed is written** — the sweep stops
  planting suggestions as a side effect of being pressed.
- A caller with **no Gmail grant** keeps today's path over the `emails` table.

Unchanged: the model still only proposes, ids are still validated against the batch,
confirmation stays on the existing confirm/reject endpoints, and there is still no calendar
code.

No breaking changes to any endpoint's shape.

## Capabilities

### New Capabilities
<!-- None. This changes how an existing capability finds its candidates. -->

### Modified Capabilities
- `application-mail-recall`: candidates come from a gated Gmail search rather than the
  stored mail table; a proposal is an unstored message; the cap and ordering requirements
  are replaced.

## Impact

- **`internal/mailrecall`**: a second candidate source behind the existing `Store` seam, or
  a sibling interface; the net's window/cap/order constants lose their reason to exist in
  the Gmail path.
- **`internal/gmailsync`**: a per-employer search + single-message fetch, beside the
  existing sync reader. This is where a Gmail credential is already handled.
- **API**: `POST /me/tracking/:slug/mail-recall` keeps its shape; proposals carry a Gmail
  message id rather than an `emails.id`, so `POST /me/emails/:id/confirm` needs an import
  step ahead of it — most likely a new endpoint that imports-and-links in one call, in the
  manner of `POST /me/emails/:id/application`.
- **Web**: the Emails tab's Link button targets the new call.
- **No migration.** The message a caller confirms is stored by the existing ingest path.
- **Unchanged**: `internal/maillink`, `internal/mailmatch`, `internal/mailclassify`,
  `internal/calmatch`, `internal/calsync`.
