## Context

The directory (`ListPublishedMentors`) already reads as "NULL means unfiltered" for
`company_slug`/`topic`/`language`, and the handler already whitelists the query
vocabulary against `knownMentorParams`, reporting anything else in
`meta.ignored_params`. The frontend already splits filtering into two halves: the
options a control offers are derived client-side from the unfiltered directory response
(`mentorFilterOptions`), while the actual narrowing happens server-side via query
params the route re-fetches with. This change adds three more entries to both halves
without changing the shape of either.

`internal/dict/vocab.SeniorityValues` already exists and is already imported from two
other `engage`-block packages (`companyfeedback`, `processreport`), each validating a
submitted value with `slices.Contains(vocab.XValues, value)` and refusing anything else.
This change follows that exact pattern rather than introducing a new one.

## Goals / Non-Goals

**Goals:**
- Three new directory filters (search text, seniority, no-reviews-yet), each following
  the existing "empty/false means unfiltered" rule.
- Seniority stored on the profile using the platform's one seniority vocabulary, never a
  new one.

**Non-Goals:**
- No ranking or sort-order change to the directory — the three filters narrow the same
  unordered result the directory already returns.
- No badges, no pricing signal, no "years of experience" as a free-text or numeric
  field — seniority is the platform's existing closed vocabulary or nothing.
- No change to how `mentorFilterOptions` decides what a dropdown offers (derived from
  the unfiltered directory) — seniority options are derived the same way topics and
  languages already are, so a seniority level nobody has stated does not appear as a
  choice.

## Decisions

### Seniority reuses `vocab.SeniorityValues`, not a new mentor-only list

The alternative — a smaller, mentor-specific list — was considered and rejected. This
codebase's "one vocabulary" convention (documented in the root AGENTS.md's Conventions
section, holding for company legal-form normalization) exists because divergent copies
of an enum drift silently; a second seniority list here would be exactly that. Reusing
`vocab.SeniorityValues` means the directory filter, the profile field and every other
consumer of seniority elsewhere in the codebase agree on one spelling by construction,
and needs no new dictionary file.

### `seniority` is a plain column, not folded into `topics`

`topics` is a free-form, open tag list the mentor writes themselves; seniority is a
closed enum with exactly one valid value at a time. Storing it as just another topic
string would let a mentor type `"senior"` as a topic (indistinguishable from the real
field to a directory search) and would need per-value validation logic to live inside
the topics list instead of being one column with one check.

### Search matches name and headline, unescaped, exactly like existing text search

`ListMentors`' new `q` parameter is implemented the same way `companies.sql`'s and
`gmail.sql`'s existing searches are: `ILIKE '%' || sqlc.arg(q) || '%'` with no escaping
of `%`/`_` in the caller's input. This is not a new decision — it is following the
codebase's already-established convention for this exact kind of search, which this
change does not have standing to revisit on its own.

### The "no reviews yet" toggle reads the existing rating aggregate, not a booking count

`ListPublishedMentors` already LEFT JOINs a per-mentor `(rating_count, rating_avg)`
aggregate from `mentor_reviews` for the card's own display. "No reviews yet" is
`COALESCE(rating_count, 0) = 0` against that same join — no new join, no new column, and
the toggle's own name matches exactly what it measures (reviews, not raw booking
counts, which the directory query has no cheap access to).

## Risks / Trade-offs

- **[Risk]** An unescaped `ILIKE` lets a caller's `q` contain `%`/`_` wildcards, which
  can widen a match past a literal substring (e.g. `q=a%b` matches more than the literal
  string `a%b` would). → Not mitigated here: this is the pre-existing, already-shipped
  behavior of every ILIKE-based search in this codebase, and changing that convention is
  out of this change's scope.
- **[Risk]** A mentor's `seniority` predates this column's write path only through new
  submissions and edits — every existing profile starts with it unset. → Accepted: the
  spec makes unset the normal, always-valid state ("does not match a directory search
  narrowed by seniority" is not an error), so no backfill is needed and none is
  attempted.

## Migration Plan

Additive only: `mentors.seniority text NOT NULL DEFAULT ''` (empty string is "unset",
matching this table's own `bio`/`meeting_url` convention of no third NULL state where a
default already means the same thing). Filed as the next available migration number.
No backfill; no reindex needed (this directory is served from Postgres, not
Meilisearch).
