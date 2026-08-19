# duplicate-marker-ownership Specification

## Purpose
Keep the three dedup passes from overwriting each other's verdicts. Each writes its own
marker column — `duplicate_of_aggregator`, `duplicate_of_role`, `duplicate_of_fuzzy` — and
`jobs.duplicate_of`, the column every reader consumes, is derived from them by a trigger.

The rule exists because one column with three writers was not a hypothetical: the role
recompute derived `duplicate_of` from scratch over role clusters and wrote NULL to rows the
other two passes had marked for entirely different reasons, which they then re-applied later
in the same run. On prod that was ~950,000 rewritten rows per cycle, six cycles a day, and an
hour per cycle in which hundreds of thousands of duplicates stood as canonical — long enough
for a scheduled rebuild to index them. Ownership took it to 13,385 rows a cycle.

Ordering between the passes survives as a cost and merge-quality rule, not as what makes the
end state correct.
## Requirements
### Requirement: Each dedup pass owns exactly one marker column

Each dedup pass SHALL write its verdict to a column no other pass writes, so that a pass
can never clear or overwrite a marker it did not set. The role-cluster recompute owns
`duplicate_of_role`, the cross-source aggregator suppression owns
`duplicate_of_aggregator`, and the fuzzy-description collapse owns `duplicate_of_fuzzy`.
A pass recomputing its own column from scratch is expected and correct; a pass reaching
another pass's column is a defect.

#### Scenario: The role recompute leaves a suppression alone

- **WHEN** an aggregator posting is suppressed against its ATS twin, and the role-cluster
  recompute then runs and finds that posting to be a singleton in its own role cluster
- **THEN** the recompute writes NULL only to `duplicate_of_role`, the suppression in
  `duplicate_of_aggregator` is untouched, and the posting stays a duplicate

#### Scenario: The role recompute leaves a fuzzy collapse alone

- **WHEN** the fuzzy pass has collapsed a set of near-identical reposts onto a canon, and
  the role-cluster recompute then runs over the same company
- **THEN** the reposts keep pointing at the fuzzy canon and the recompute writes nothing to
  `duplicate_of_fuzzy`

### Requirement: The effective duplicate marker is derived, not written

The system SHALL derive `jobs.duplicate_of` from the three owned columns rather than accept
direct writes to it, resolving them as `COALESCE(duplicate_of_aggregator, duplicate_of_role,
duplicate_of_fuzzy)` — the order that reproduces which pass wins a contested row today, with
the only heuristic pass last. Every existing reader of
`duplicate_of` SHALL keep its current meaning: a non-NULL value names this posting's canon,
and NULL means the posting is itself canonical.

#### Scenario: Readers are unchanged

- **WHEN** any consumer of duplicate status runs — job search, the facet index claim, the
  semantic outbox, enrichment eligibility, pruning, or cluster copies
- **THEN** it reads `jobs.duplicate_of` exactly as before and observes the same value it
  would have observed under the single-column scheme

#### Scenario: A contested posting keeps the verdict it has today

- **WHEN** an aggregator posting is both suppressed against its ATS twin and a member of a
  role cluster, so it carries a marker in two owned columns
- **THEN** `duplicate_of` resolves to the aggregator verdict, pointing at the first-party ATS
  posting rather than at the role canon

#### Scenario: A direct write does not survive

- **WHEN** code sets `jobs.duplicate_of` directly instead of naming an owning column
- **THEN** the derivation overrides it, so the stored value always reflects the owned
  columns and no writer can put the row into a state the passes disagree with

### Requirement: A marker refresh is idempotent and order-independent

A full marker refresh over an unchanged catalogue SHALL write zero rows, and the passes
SHALL produce the same end state regardless of the order they run in. Pass order MAY be
retained as a cost optimization, but SHALL NOT be load-bearing for correctness.

#### Scenario: Re-running the whole refresh writes nothing

- **WHEN** the marker refresh runs twice over a catalogue with no new, changed, or closed
  postings
- **THEN** the second run reports zero rows re-marked for all three passes

#### Scenario: An interrupted refresh leaves no duplicate unmarked

- **WHEN** a marker refresh fails or is killed after one pass has run and before the others
- **THEN** every marker set by a pass that did not run this cycle is still in place, and no
  posting that was a duplicate before the run has become canonical because of it

#### Scenario: A concurrent rebuild sees a consistent catalogue

- **WHEN** a full facet rebuild scans the catalogue while a marker refresh is running
- **THEN** no posting is indexed as canonical solely because a pass had cleared its marker
  and not yet restored it

