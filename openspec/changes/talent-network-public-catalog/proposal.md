## Why

The Talent Network ships a per-candidate public page and nothing that leads to it.
There is no entry in `web/src/lib/accountNav.ts` and no button on `/my/profile`, so
`/my/talent-network` — the page that owns the opt-in — cannot be reached with a mouse.
The result is a feature with a backend, a frontend and, in practice, no members.

Being found should not require the candidate to hand out a link themselves. This change
adds the surface that makes the network worth joining: one public, unauthenticated
catalogue of the people who opted in, anonymised, and one button that puts a candidate
in it.

Anonymity is the product decision, not a mode: a candidate's stated fear is their current
employer discovering they are looking. The public tier therefore shows a de-identified
professional card and nothing that names a person or a company. A later change adds the
approved-recruiter tier that sees the rest; nothing here builds toward it beyond leaving
the seam.

## What Changes

- **BREAKING (feature-scoped): the three visibility modes collapse to two.**
  `users.talent_network_visibility` keeps `off` and `anonymous`; `public` is retired.
  A migration rewrites existing `public` rows to `anonymous` and narrows the CHECK
  constraint. The candidate's choice becomes one toggle — in the network, or not —
  because a mode picker asks a question the product should answer itself.
- **New public catalogue**, unauthenticated and rate-limited: `GET /api/v1/talent`
  (filtered, paged list) and `GET /api/v1/talent/{handle}` (one card).
- **A minted catalogue handle** replaces the uuid in the public URL: `/talent/backend-7f2a`
  rather than `/talent-network/<uuid>`. It is minted once, when the candidate first joins,
  and never changes — not even through a round trip out of the network. It is **not** the
  account's `username`: `username.Suggest` derives that from the email's local part, so for
  most accounts it *is* the person's name, and it is also their hosted mailbox address.
  Putting it in the URL of a page that promises to withhold the name would undo the
  feature in the address bar.
- **A new projection, `Structured.Catalog()`**, that is a whitelist of *structured*
  fields only. It withholds the name, photo, contacts, **every company name** — not just
  the current one — and **every free-text field** (summary, highlights, project
  descriptions). Prose names the employer in fields no column-level mask covers, so the
  existing `Anonymous()` is not safe enough for a page anyone can crawl.
- **The card's job title is derived, not quoted.** `headline` is free text and carries
  values like "Backend @ <employer>", so the card's heading is built from the most recent
  role's title plus the seniority `internal/dict/classify` resolves for it.
- **Filters over structured facets only**: category and seniority (from `classify`),
  skills, timezone region, city (`users.resume_cities`), years of experience, languages.
- **The opt-in becomes reachable**: a `Talent Network` entry in the account navigation and
  an invitation block on `/my/profile`.
- **SEO**: the list is indexable; an individual card carries `noindex`, so leaving the
  network is not undone by a search engine's cache.
- Not included, deliberately: recruiter accounts and their approval, any reveal of a
  candidate's identity, contact details in any tier, and a mailing to the existing
  members-by-default population.

## Capabilities

### New Capabilities

- `talent-network-membership`: the candidate's single opt-in — what joining means, what
  the two states are, and where the control is reachable from.
- `talent-network-catalog`: the public, unauthenticated catalogue — who appears in it,
  what one card may and may not carry, how it is filtered, ordered and paged, and how it
  is protected from bulk extraction.

### Modified Capabilities

None. The Talent Network's existing behaviour lives in
`openspec/changes/talent-network-profile-visibility`, which has never been archived, so
there is no `openspec/specs/talent-network-profile/` to write a delta against. That
change's tri-state requirement is superseded by `talent-network-membership` here; see
Impact.

## Impact

**Schema.** Three migrations on `users`, each doing one thing:

- rewrite `talent_network_visibility = 'public'` to `'anonymous'` and narrow
  `users_talent_network_visibility_check` to two states. `users` is hot, so the constraint
  swap follows the split `ADD ... NOT VALID` + `VALIDATE` shape migration 0085 documents.
- add `talent_handle text` (nullable — minted on first join, so a non-member has none).
- build its unique index `CONCURRENTLY`, in its own `no-transaction` file, the shape 0086
  uses. **Not `IF NOT EXISTS`**: that skips the invalid carcass a cancelled
  `CONCURRENTLY` build leaves behind, which is exactly what migrations 0117 and 0118
  existed to repair.

`talent_network_public_id` is retired with the route that served it. Two public
identifiers for one page is a drift waiting to happen — one of them eventually gets
handed out where the other was meant to be — and nothing has ever shared the uuid, because
the control that produces it has never been reachable.

**Go.**
- `internal/candidate/resumeextract/visibility.go` — add `Catalog()` beside `Anonymous()`
  and `Public()`. `Public()` becomes unreachable from production code once the mode is
  retired; it is removed in the same change rather than left as a projection nothing may
  select.
- `internal/candidate/talentnetwork/` — new package (block `candidate`, layer 4): the
  list query, the filter vocabulary, paging, and the card assembly that reaches
  `internal/dict/classify` (layer 2). **It must be added to the table in
  `internal/platform/arch/layering/blocks.go`** or both layering guards fail.
- `internal/platform/db/queries/users.sql` — the list and count queries and the
  by-`public_id` read; `make sqlc` regenerates.
- `internal/api/handler/` — the two public routes, wired through `internal/api/ratelimit`.

**Web.**
- New `web/src/routes/talent/` (list) and a restyle of the existing
  `web/src/routes/talent-network/[publicId]/` to the catalogue card.
- `web/src/routes/my/talent-network/+page.svelte` loses the three-way picker.
- `web/src/lib/accountNav.ts` gains the section; `web/src/routes/my/profile` gains the
  invitation block.
- Ported from the sibling `freehire-recruit` repository rather than rewritten:
  `CandidateCard.svelte`, `candidateQuery.ts`, `timezoneCountry.ts` (the
  timezone→ISO country table behind the flags).

**Storage.** Postgres only. Meilisearch is not involved: the population is in the
hundreds, and an index here would add a drain, a rebuild and a second way to serve stale
people. The seam is noted in the package doc, not built.

**Related work.** The unarchived `talent-network-profile-visibility` and
`talent-network-entry-redesign` changes describe the surface this replaces. Their task
lists are already `[x]` and their `tasks.md` no longer matches the code (the overlay
panel they describe became `/my/talent-network`); reconciling and archiving them is part
of finishing this change.
