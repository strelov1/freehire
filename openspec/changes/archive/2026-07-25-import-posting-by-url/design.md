## Context

Three mechanisms in this repo turn a link into catalog content, and they are not
interchangeable:

- `internal/contribution` — records a **board** (`source`, `board`) from a pasted link,
  network-free where possible, rewarded with AI credits. One board yields many vacancies,
  forever, once ingest onboards it.
- `internal/submission` — a **vacancy typed in by a human**, held in a moderation queue and
  minted into a live posting on approval.
- `internal/linksource` + `cmd/resolve-url` — a **machine import of one page**: fetch,
  parse, and write it under the destination's own `(source, external_id)`, no human in the
  loop.

The extension needs the third. Its user is standing on a specific vacancy that we do not
carry, and the answer they want is "it is in the catalog now, here it is" — the same
answer `cmd/resolve-url` gives an operator today.

## Goals / Non-Goals

**Goals:**
- One HTTP call takes a page URL and returns the catalog posting for it — found or
  imported.
- Never mint a duplicate of a posting we already carry.
- One definition of "import a vacancy from a link", shared by the CLI and the endpoint.
- A page we cannot parse is not lost: it lands in the existing triage queue.

**Non-Goals:**
- Rewriting the credit economy, or gating imports behind moderation.
- Teaching new ATS adapters — that stays the per-provider work `linksource` was built for.

## Decisions

### The catalog is consulted before the network

The first thing the endpoint does is the `/jobs/find` lookup: identity from the URL, then
the stored-URL comparison. Only a miss reaches `linksource`. Two reasons, and the second is
the important one:

- A page we already carry costs no outbound fetch.
- The generic resolver writes under source `weblink`, whose `external_id` is the page URL.
  A himalayas posting imported that way would NOT dedup against its himalayas twin (that
  row's identity is the same URL under a different source), so the catalog would grow a
  second copy of a posting it already had — invisible to the ingest dedup passes, which
  work per company+title, not per URL. Checking first is what makes the endpoint safe to
  point at any page.

### The import path is lifted out of cmd/resolve-url verbatim

`cmd/resolve-url` already encodes decisions that took work to get right: the generic
resolver appended after the host-scoped adapters and never in the shared registry; the
write through `job.New` → `UpsertJob` in one transaction with the enrichment enqueue; the
best-effort index push that skips duplicates and closed rows. Reimplementing that in a
handler would fork it.

So it moves to `internal/linkimport`, and the command becomes a CLI over the package. The
package owns the registry construction and the write; the caller supplies the pool,
queries and an optional search client. No behavior changes — the tests that cover the
command's write path move with it.

### Three outcomes, three status codes

| Outcome | Status | Body |
| --- | --- | --- |
| Already in the catalog | 200 | `{"data": {"public_slug": "…", "status": "found"}}` |
| Parsed and written | 201 | `{"data": {"public_slug": "…", "status": "imported"}}` |
| Not parseable, queued for triage | 202 | `{"data": {"public_slug": null, "status": "queued"}}` |

The panel needs to distinguish "here is your card" from "thanks, a human will look", and
the slug is what it needs in the first two cases. `status` is carried in the body rather
than left implicit in the code so a client that ignores status codes still behaves.

A URL that is not http(s) is 422, matching the contribution endpoint's answer to garbage.

### The rate limiter is shared with contributions, deliberately

`contributionLimiter` exists because "an endpoint that makes the server fetch a
user-supplied URL is an outbound-fetch amplifier and a timing oracle". That is exactly
this endpoint. Mounting a second independent limiter would double the budget for the same
abuse; so the limiter is built once in the route wiring and mounted on both routes,
capping a user's outbound-fetch-triggering calls as a whole.

## Risks / Trade-offs

- **A parsed page that is not a vacancy.** The generic resolver only accepts a page with a
  `JobPosting` ld+json block and a title and company, and answers `ok=false` otherwise —
  the same gate `cmd/resolve-url` relies on. A site that publishes JobPosting markup on a
  listing page could import noise; the `weblink` lifecycle (liveness-probed, closed when
  dead) bounds how long such a row survives.
- **Imports are unreviewed.** Deliberate: they are machine-parsed from structured markup,
  not user-typed prose. If abuse shows up, the queue exists (`job_submissions`) and the
  endpoint can be moved behind it without a wire change.
- **`weblink` postings have no board to re-crawl them.** Already true of every
  `cmd/resolve-url` import.

## Open Questions

None.
