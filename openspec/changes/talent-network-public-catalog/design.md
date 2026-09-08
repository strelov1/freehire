## Context

The Talent Network's opt-in (`users.talent_network_visibility`, migration 0085) is a
tri-state: `off` / `public` / `anonymous`. `public` shows the candidate's name and every
employer; `anonymous` masks only the *current* employer's `company` column
(`Structured.Anonymous()`). Both serve one page per candidate, reached by an opaque uuid.

Nothing links to any of it. `web/src/lib/accountNav.ts` has no Talent Network section and
`/my/profile` has no entry point, so the toggle is unreachable and the member count is
effectively zero. The population it would draw from is known from the sibling
`freehire-recruit` repository's own measurements: ~697 accounts have uploaded a CV and
~579 have a structured extract that still matches it.

This change replaces the per-candidate link with a catalogue, and the mode picker with a
single opt-in. It builds the public tier only. The approved-recruiter tier that sees
names and employers is a separate change; the seam is the projection boundary, not a
flag.

Constraints that shape the work:

- The catalogue is served to anonymous visitors and is therefore crawlable by definition.
  ClaudeBot and friends are already the majority of this host's traffic.
- `internal/platform/arch/layering` enforces the block table. The catalogue is
  candidate-shaped (layer 4) and may reach `dict` (layer 2); nothing below may reach it.
- `users` is one of the hottest tables in the app; a constraint swap on it needs the
  split `NOT VALID` + `VALIDATE` shape migration 0085 already documents and explains.

## Goals / Non-Goals

**Goals:**

- One public, unauthenticated, filterable catalogue of opted-in candidates.
- A public projection that cannot leak a person's identity or an employer's name, and
  whose safety is a property of the projection's *shape*, not of a reviewer's diligence.
- One reachable control that puts a candidate in the catalogue or takes them out.
- A card readable enough that a recruiter can tell whether to want more.

**Non-Goals:**

- Recruiter accounts, their approval, and the fuller view they get. Later change.
- Any reveal of a candidate's name, employer, or contact details, to anyone, in any tier
  this change ships.
- Notifying or enrolling the ~579 existing CV holders. Membership starts empty and grows
  by the button.
- Search relevance, semantic matching, or Meilisearch. Filters are exact-facet only.

## Decisions

### The public projection emits dictionary terms, numbers and dates — never CV prose

`Catalog()` is a whitelist, and the whitelist admits no free text at all. Every string it
emits is either a value resolved by a dictionary (`internal/dict/classify` for
seniority and category, `internal/dict/skilltag` for skills and stack) or a formatted
date; everything else is an integer.

Kept: `total_years`, `languages`, dictionary-resolved `skills`, `certifications`,
education `degree` and `year`, and per role: the classified seniority/category pair, the
period, and the dictionary-resolved stack.

Withheld: `full_name`, `headline`, `location`, `email`, `phone`, `links`, `summary`,
every experience entry's `company`, `location`, `summary` and `highlights`, every
education entry's `institution`, and `projects` entirely.

*Alternative considered — reuse `Anonymous()`.* Rejected. It masks the `company` column of
current roles and nothing else, which is right for a page the candidate hands to one
person and wrong for a page a crawler reads. A candidate writes "at <employer> I rebuilt
the billing pipeline" into `summary`, and the column-level mask never sees it.

*Alternative considered — pass the raw job title through.* Rejected for the same reason at
smaller scale: titles carry "Backend Engineer @ <employer>" often enough to matter. Passing
it through `classify.Parse` turns it into a value from a closed vocabulary, which cannot
carry a name it was not given.

*Alternative considered — drop `institution` but keep `projects`.* Rejected. A project's
`name` is frequently the employer's product, and its `highlights` are prose.

The rule is what makes this testable: **a public response contains no substring that was
not either a dictionary term, a number, or a date.** A test can assert that over real
fixtures; "we remembered to mask the right fields" cannot be.

*Trade-off, stated plainly:* the public card is austere. That is the point of the tier,
and richness is what the recruiter tier will be worth paying for.

### Two states, not three — and `Public()` goes with the mode

The picker becomes a toggle. `public` is retired, existing rows are rewritten to
`anonymous`, and the CHECK narrows. `Structured.Public()` and the handler branch that
selects it are deleted in the same change: a projection no caller may reach is not a
seam, it is a loaded gun, and `deadcode` would report it anyway.

*Alternative considered — keep `public` as a second, name-showing tier.* Rejected: it asks
the candidate to reason about a disclosure trade-off at the moment they are least equipped
to, and it splits the catalogue's population for no benefit this change can name.

### The catalogue is projected in memory, on a TTL, and served from that snapshot

`internal/candidate/talentnetwork` reads the opted-in rows that pass the stamp gate,
projects each through `Catalog()`, and holds the result as an immutable snapshot refreshed
on a TTL. Filtering, ordering, counting and paging all run against the snapshot.

The reason is not performance, it is honesty: category and seniority do not exist as
columns — they are derived by `classify` at read time — so a SQL `WHERE category = ?`
would require storing them, backfilling them, and keeping them in step with a dictionary
that changes weekly. At a few hundred members the whole catalogue is a few megabytes and
the derivation is microseconds.

*Alternative considered — derived columns plus a backfill worker.* Rejected as
infrastructure ahead of need. *Alternative considered — Meilisearch.* Rejected: an index,
a drain, a rebuild and a second way to serve someone who has already left.

**The seam is stated in the package doc:** when the membership outgrows a snapshot that
fits comfortably in memory (order of thousands), the projection moves to a table written
by a worker, and the query moves to SQL. Nothing in the handler or the wire shape has to
change for that.

### The order is total, and paging is offset-based

`ORDER BY` freshness of the structured extract, tie-broken by `talent_network_public_id`.
The tie-break is load-bearing: without it two members sharing a timestamp order
arbitrarily, and an arbitrary order across `LIMIT`/`OFFSET` silently drops some people and
repeats others. The sibling repository's query carries the same tie-break for the same
reason.

### Membership is the gate, and only membership

One predicate decides who is in the catalogue: `talent_network_visibility <> 'off'`, AND
the stamp gate (`resume_structured_uploaded_at = resume_uploaded_at`, both non-null). The
stamp gate is not a quality filter — it is the same "this structure still describes the CV
on file" rule the rest of the product uses, and without it most cards would say the CV is
still being processed.

### The list is indexable; a card is not

The catalogue page is a legitimate landing page and carries no personal data. An
individual card is a person, and a search engine's cache would outlive their decision to
leave. So: the list is indexed, each card carries `noindex`, and the card's response
carries a short `Cache-Control` so an intermediary does not hold it either.

### Both routes are rate-limited

`internal/api/ratelimit` covers both, on the real routes rather than a route group that
might not contain them. The catalogue is small, complete and machine-readable — exactly
the thing worth scraping in one evening — and a limit is the only thing between it and a
copy.

## Risks / Trade-offs

- **The catalogue launches empty.** → The nav entry and the `/my/profile` invitation block
  are in this change's scope, not a follow-up. Without them nothing changes at all. A
  mailing to existing CV holders is the growth lever and is deliberately separate, because
  it is a consent conversation, not a feature.
- **An austere card may read as a thin product.** → Accepted. The list makes its value
  from *volume and filters*, not from any one card; the depth is the recruiter tier's
  reason to exist.
- **`classify` resolving nothing for a title leaves a role with no heading.** → The role
  still renders with its period and stack under a neutral label. Silently dropping the
  entry would make a work history look shorter than it is.
- **Retiring `public` changes what an existing member agreed to.** → It narrows what is
  shown, never widens it: a `public` row becomes `anonymous`, so the change can only
  disclose less. No notice is owed for a reduction, and the count is small.
- **The snapshot serves a departed member for up to one TTL.** → The TTL is short
  (minutes) and the by-id route re-checks membership against the database before
  answering, so a direct link stops working immediately even while the list lags.
- **In-memory projection over every member on refresh.** → Bounded by the same membership
  predicate, measured in the package's own test, and the seam above says what to build
  when it stops being true.

## Migration Plan

1. Migration lands and is applied before the code that reads it (the ordinary rule for
   this repository): rewrite `public` → `anonymous`, drop and re-add the CHECK using
   `NOT VALID` + `VALIDATE`, outside a transaction, per 0085's precedent.
2. Deploy. The catalogue is empty and correct; `/my/talent-network` shows a toggle.
3. Rollback is the ordinary app rollback. The migration is not reverted: the narrowed
   constraint is satisfied by every row the old code could write except `public`, and no
   deployed code writes `public` after step 2.
