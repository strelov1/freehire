## Context

`internal/dict/location/eligibility.go` already matches `"secret clearance"` and
`"ts/sci"`, but spends them as a *geography* hint: when the location dictionary
leaves a posting unpinned, a US-clearance phrase pins it to the US. The file's own
comments record why UK clearance vocabulary was left out — `SC`, `DV`, and `BPSS`
are short tokens that collide with ordinary words, and for geography the collision
cost was not worth the rescue.

A dedicated facet changes that calculus. It does not need to infer a *place*, only
a *requirement*, so it can accept a phrase like `SC clearance` that would be a poor
geography anchor. The two consumers stay separate: geography keeps its list,
clearance gets its own.

`is_tech` is the closest structural precedent — a tri-state boolean, dictionary-
derived, stored as a `jobs` column, served top-level in `jobview`, filterable in
Meilisearch, re-derivable by `cmd/backfill-derive`. This change follows its path
step for step, which is what keeps it small.

The single hard constraint comes from `internal/search/search/client.go:565`: a
binary that requests a filterable attribute the live index has not declared
hard-500s `/api/v1/jobs/facets` for every caller. Ordering is not a nicety here.

## Goals / Non-Goals

**Goals:**

- A searcher can exclude postings that require a government clearance, and a
  cleared searcher can select them.
- Detection is deterministic and auditable — a phrase list a human can read,
  diff, and correct.
- The existing catalogue gets the facet without a 15-hour job or a de-TOASTing
  table scan.
- Precision over recall: a false positive hides a job the candidate could have
  got, which is strictly worse than leaving one unmarked.

**Non-Goals:**

- **Clearance levels.** No `uk_sc` / `us_ts_sci` enumeration. A boolean answers
  the question asked; a level vocabulary can be added later behind the same column
  name without breaking the wire contract.
- **Citizenship and work authorisation.** `location.EligibilityFromDescription`
  already turns those into geography facets. Folding them in here would blur what
  the facet means and duplicate a working filter.
- **Sponsorship.** `enrichment.visa_sponsorship` is a separate, LLM-derived facet
  about employer paperwork, not candidate vetting.
- **Retro-fixing the geography anchors.** The clearance list is new and separate;
  `eligibility.go`'s list is not touched.

## Decisions

### Detection lives in `internal/dict/location`, not a new package

`clearance.go` sits beside `eligibility.go`. It reuses the word-boundary matcher
and the negation guard that file already owns and tests — `negationWords`,
`isWordByte`, and the assertion walk. A new package would either duplicate those
or force them public.

*Alternative considered:* a new `internal/dict/clearance` package. Rejected: the
matcher primitives are unexported in `location`, and the facet is a sibling of the
eligibility signal in both provenance (description prose) and discipline
(anchored, negation-aware, precision-first). If clearance later grows levels and
scheme metadata, promoting it to its own package is a mechanical move.

*Layering note:* `dict` is layer 2 and may import only `platform`. Nothing here
reaches upward, so the depguard table needs no new entry — the package already
exists in `internal/platform/arch/layering/blocks.go`.

### Two rules, not one: anchored phrases plus a labelled field

The sample showed the phrase list alone misses about a fifth of true positives,
because ATS postings state the requirement as a field rather than a sentence:
`Clearance: Secret`, `CLEARANCE REQUIRED FOR START: Yes`,
`Clearance Level: Public Trust`. The labelled-field rule reads `clearance`
(optionally followed by `level` / `required` / `type`), a separator, and then the
value that follows it, marking the posting when that value names a scheme or
asserts the requirement, and declining when it denies one (`No`, `None`, `N/A`).

*Alternative considered:* widening the phrase list until it covers the field forms.
Rejected: the field's value varies without bound (`Secret`, `TS with SCI
eligibility`, `Ability to Obtain Public Trust`, `Yes`), so a phrase list would
chase a long tail forever while the structure itself is trivially recognisable.

### `public trust` is only ever matched as `public trust clearance`

Measured directly: of a 127-row sample containing both words, most were "commitment
to public trust", "build public trust", or public-relations copy. As a bare phrase
it is a promotional cliché; as `public trust clearance` it is a US vetting tier.
The same reasoning keeps `active clearance` in and a bare `clearance` out.

### Storage is a nullable column, and no explicit `false` is ever written

`jobs.requires_clearance boolean`, nullable, mirroring `is_tech`. `true` means
detected; `NULL` means the dictionary said nothing — including the case where a
denial cancelled an anchor.

Writing `false` on a denial was considered and rejected. It would imply the system
can tell "this posting promises no clearance is needed" from "this posting is
silent", and only ~600 postings state the denial explicitly, so the distinction
would be true for a rounding error of the catalogue and misleading everywhere
else. `requires_clearance=false` on the API therefore means *not marked*, which is
exactly what the asking user wants.

*Migration shape:* `ALTER TABLE jobs ADD COLUMN requires_clearance boolean`.
Nullable with no default, so Postgres takes no table rewrite and no long lock.

### The filter maps to the search layer like `is_tech`

`query_filter.go` already carries `"is_tech": "is_tech"` for a top-level served
facet. `requires_clearance` joins it the same way, and the attribute is added to
the filterable set in `client.go`.

`requires_clearance=false` cannot be a plain equality filter, because the stored
value is `NULL` for most of the catalogue and Meilisearch documents omit the field
entirely rather than carrying a null. The filter for "exclude clearance jobs" is
therefore `requires_clearance IS NULL OR requires_clearance = false` in
Meilisearch's filter syntax — expressed once, in the mapping layer, not in every
caller.

### Backfill names its candidates through Meilisearch

`description` is a searchable attribute, so the index can produce the candidate
ids in seconds. The backfill queries the `clearance` token plus the anchors that
do not contain the word (`ts/sci`, `polygraph`, `bpss`, `vetting`, `agsva`),
unions the ids, and re-derives only those rows.

Over-fetching is deliberately cheap: the matcher, not the query, decides, and a
candidate it declines simply keeps `NULL`. That makes the search query a *recall*
device with no precision obligation, which is the right division of labour.

*Alternative considered:* `cmd/backfill-derive` over the catalogue. Rejected on
two measured grounds — it runs ~15 hours, and a `description` predicate over the
full table de-TOASTs 8M rows, a trap this repo has already been bitten by.

## Risks / Trade-offs

**A false positive hides a job the candidate could have got** → The highest-cost
failure. Mitigated by anchoring every phrase, banning bare short tokens, matching
on word boundaries, and letting a denial cancel the whole description. Recall is
knowingly sacrificed: an unmarked clearance job merely leaves the status quo.

**The rollout can 500 `/api/v1/jobs/facets` for everyone** → Ship the Meilisearch
settings patch before the binary, and verify against the live index that the
attribute is declared before deploying. This is step-ordered in the migration plan
below, not left to judgement on the day.

**The labelled-field rule reads an unbounded value** → It is bounded by the two
directions it can go: it marks on a scheme name or a "required/yes" assertion, and
declines on an explicit denial. Anything else leaves the posting unmarked, which
is the safe default.

**`polygraph` outside a vetting context** → Measured at 83% exact-phrase presence
in its sample with every inspected hit in a defence-contractor posting. Accepted:
the word has essentially no other use in job copy.

**The estimate could be wrong** → It is a sample-based extrapolation, and the
proposal says so and shows the method. The facet's value does not depend on the
exact figure; the backfill will produce the true count, and that number is worth
recording once it exists.

## Migration Plan

1. **Migration 0119** — `ALTER TABLE jobs ADD COLUMN requires_clearance boolean`.
   Additive and nullable; safe to apply ahead of the code.
2. **Meilisearch settings patch** — declare `requires_clearance` filterable on the
   live index, and verify by reading the settings back. **Before the binary.**
3. **Deploy the binary** — new postings get the facet from ingest onward.
4. **Backfill** — the candidate-set pass. Idempotent, resumable, safe to stop.
5. **Record the true count** once the backfill completes.

**Rollback:** the facet is additive. Dropping the filter from the UI hides it;
leaving the column populated costs nothing. The Meilisearch attribute can stay
declared safely — the hazard runs in the other direction only.

## Open Questions

None blocking. One to revisit after launch: whether the marked postings warrant a
clearance *level* vocabulary. The `Clearance: <value>` field already carries the
level verbatim, so the data to answer it will exist as a by-product of this change.
