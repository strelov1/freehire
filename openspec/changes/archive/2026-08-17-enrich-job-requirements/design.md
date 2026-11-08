## Context

See proposal.md - Why. Relevant existing structure: the typed `Enrichment`
contract (`internal/enrich/enrichment.go`) is the schema's source of truth for
`jobs.enrichment` JSONB; the LLM request schema is generated from it via
reflection (`internal/enrich/schema.go`, `internal/llmschema`); a hand-written
tolerant-decode shadow struct (`internal/enrich/enrichment_unmarshal.go`)
guards against the model returning a field in the wrong JSON shape; and
`internal/jobview` embeds the raw `Enrichment` wholesale into the public job
object, so any field added to the contract is served unless explicitly folded
out.

`internal/matchanalysis` already has an established, working precedent for
exactly this shape — `Requirement{Text, Priority}` with a `coercePriority`
post-hoc fallback — but it is not schema-constrained at all (that package
issues plain prompt text, no `internal/llmschema` involvement), because its
LLM calls don't use structured-output schemas the way `internal/enrich` does.

## Goals / Non-Goals

**Goals:**

- Extract a job-only, freeform, structured requirements list at enrich time
  and store it in the existing JSONB payload.
- Reuse the established bounding/sanitizing pattern for free-text enrichment
  fields (`boundCities` et al.) and the established priority-coercion
  pattern from `matchanalysis.coercePriority`, without creating a dependency
  between the two packages.

**Non-Goals:**

- Wiring the new field into `internal/hardconstraint` or
  `internal/matchanalysis` (a deliberate, separate follow-up).
- Any change to `internal/jobfacts`' deterministic category matchers.
- Deciding whether/when to bump `enrich.Version` to retroactively re-enrich
  the existing catalogue with `requirements` (an operational decision, not a
  code change).

## Decisions

**Freeform `{text, priority}` shape, not pinned to `hardconstraint`'s 6
categories.** A fixed-category shape would force the model to force-fit
open-ended prose into 6 buckets, losing the specific, useful text a posting
actually states. `matchanalysis.Requirement` already validates this freeform
shape works well for LLM-extracted posting requirements; mirroring it here
(field names and semantics, not a shared Go type) keeps the two vocabularies
recognizable as siblings without coupling the packages.

**No schema-level enum constraint on `priority`.** `internal/llmschema.Enum`
constrains a top-level field, or an array field whose items are themselves
scalars (it sets the constraint on the array's `items` node) — it has no
notion of constraining one nested property inside an array-of-objects field.
Building that capability into `llmschema` for one field is disproportionate:
the identical field in `matchanalysis` ships today with no schema constraint
at all, relying purely on prompt wording plus post-hoc coercion, and that
precedent is proven. `enrichment.go`'s `Sanitize` coerces any value that
doesn't case/whitespace-insensitively match `required` down to `preferred`,
so no invalid value can survive to being served — the schema is a first
line where available, never the only line (matching this package's own
stated invariant for `Sanitize`/`Validate`).

**A defensive `requirementListOrWrap` tolerant-decode type.** The
`enrichment_unmarshal.go` shadow struct exists because strict JSON-schema
mode has *not* fully prevented the model from returning the right value in
the wrong shape for other fields (that's its whole premise). Following that
precedent, `requirements` gets the same array-or-bare-object tolerance
`sliceOrWrap` gives string fields, rather than assuming the new field is
exempt from a failure mode already observed elsewhere in this same payload.

**Let the field flow through `internal/jobview` unchanged.** `jobview`
already serves free-text/enum enrichment fields (`summary`, `visa_sponsorship`,
salary fields, etc.) with no suppression; suppressing this one specifically
would need new code and a new test to maintain, for a field that carries no
more sensitivity than the fields already served. It also directly serves the
proposal's motivation — the data needs to be inspectable somewhere, and there
is no admin/debug path in this codebase that dumps raw `jobs.enrichment`, so
`jobview` is the only realistic way to see it short of `psql`.

**No shared constant/type between `internal/enrich` and
`internal/matchanalysis`.** The two packages' `Requirement` types are
independently defined and independently bounded (mirroring, not sharing,
`matchanalysis`'s `DefaultMaxReqTextRunes`/`coercePriority`). `internal/enrich`
must not import `internal/matchanalysis` — that would invert the codebase's
existing layering (matchanalysis is a downstream consumer of enrichment
concepts, not a peer).

## Risks / Trade-offs

- [Nested-array bounding logic duplicates a pattern instead of factoring it
  out] → Acceptable: `boundCities` is already an established one-off pattern
  in this file, not a shared helper; adding a second one-off (`boundRequirements`)
  is consistent with how the file already handles multiple free-text bounds.
- [A future reader adds an `llmschema.Enum("requirements", ...)` override,
  which would silently produce an inert/wrong schema constraint rather than
  erroring] → Mitigated with an explicit code comment on the field noting why
  it is deliberately schema-unconstrained.
- [Two similarly-named, similarly-shaped `Requirement` types in the codebase
  (`enrich.Requirement` vs `matchanalysis.Requirement`) could be conflated by
  a future reader] → Mitigated with a doc comment on `enrich.Requirement`
  cross-referencing the sibling type and stating they are deliberately
  unrelated.
- [Existing already-enriched jobs won't have `requirements` until
  re-enriched] → Accepted as out of scope per proposal; `enrich.Version` bump
  is a separate operational decision.

## Migration Plan

No database migration. Deploy is a plain code change: new enrichment runs
(new jobs, or a future version bump) populate `requirements`; existing rows
simply omit the key until then, which the contract already treats as normal
(every field is optional). No rollback concern beyond a normal revert — the
JSONB column has no schema to migrate back.
