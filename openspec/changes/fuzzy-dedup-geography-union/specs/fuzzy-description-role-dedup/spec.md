## MODIFIED Requirements

### Requirement: The fuzzy pass runs after and never overrides the exact pass

The fuzzy-description pass SHALL run AFTER the exact role-cluster recompute and operate only
over rows the exact passes did not claim, so it merges what byte-exact matching left split and
never re-splits or contradicts a deterministic collapse. It SHALL write only its own marker
column, `duplicate_of_fuzzy`.

The pass SHALL be REVERSIBLE by its own next run. A row already carrying only a
`duplicate_of_fuzzy` marker SHALL remain a candidate, and the pass SHALL clear that marker
when the row no longer clusters — because its canon closed, because the descriptions diverged
below the threshold, or because an exact pass has since claimed it. Reversal is the fuzzy
pass's own duty: the role recompute writes `duplicate_of_role` and the aggregator pass writes
`duplicate_of_aggregator`, so neither can release a fuzzy marker, and the derived
`duplicate_of` keeps returning it. Without this, a fuzzy marker is permanent and the row it
hides is out of search forever.

A row whose marker is cleared SHALL re-enter the live search index through the same
duplicate→canonical transition bookkeeping that already re-queues a released role or
aggregator duplicate.

Release SHALL apply only to rows the pass actually REACHED A VERDICT ON. A row it declines to
judge keeps whatever marker it holds. The two cases must not be confused:

- A bucket the pass skips on COST — one past its size cap — is not judged. The cap exists
  because a handful of generic-title-by-location buckets carry most of the pairwise work; it is
  a compute decision, and releasing its members would un-collapse the largest groups in the
  catalogue on that basis.
- A row that cannot cluster — alone in its bucket, or with a title that normalizes to nothing —
  IS judged, and the verdict is "no cluster". This is the case that frees a marker after its
  canon closes, because the survivor is then alone in its bucket.

Running after the exact pass remains a cost optimization and a merge-quality rule: it keeps
the fuzzy pass off rows already claimed deterministically. It is no longer what makes the
end state correct.

#### Scenario: Exact-collapsed reposts are untouched

- **WHEN** the exact pass has already collapsed byte-identical-description reposts
- **THEN** the fuzzy pass leaves those `duplicate_of` markers unchanged

#### Scenario: Re-running is stable

- **WHEN** the fuzzy pass runs twice with no new postings
- **THEN** the second run changes no `duplicate_of` markers

#### Scenario: A full refresh cycle is stable

- **WHEN** the whole marker refresh runs twice with no new postings, so the role recompute
  runs between the two fuzzy passes
- **THEN** the second fuzzy pass changes no markers, and the role recompute clears none of
  the fuzzy markers the first pass set

#### Scenario: A marker is released when its canon closes

- **WHEN** the canon a posting was fuzzy-marked onto is closed, and the fuzzy pass runs again
- **THEN** the posting's `duplicate_of_fuzzy` is cleared and the posting re-enters the search
  index

#### Scenario: A marker is released when the postings diverge

- **WHEN** a fuzzy-marked posting's description is rewritten so its similarity to its canon
  falls below the threshold, and the fuzzy pass runs again
- **THEN** the posting's `duplicate_of_fuzzy` is cleared

#### Scenario: An oversized bucket keeps its markers

- **WHEN** a company+title bucket exceeds the pass's size cap, so the pass loads no descriptions
  for it
- **THEN** the markers its members carry are left untouched, rather than released as if the pass
  had found no cluster

#### Scenario: An already-marked row is still considered

- **WHEN** the fuzzy pass runs over a company whose only remaining candidates already carry
  `duplicate_of_fuzzy`
- **THEN** those rows are loaded and re-decided rather than skipped

## ADDED Requirements

### Requirement: A fuzzy-suppressed posting does not remove its geography from search

The fuzzy-description pass SHALL NOT narrow the catalogue's searchable geography. A posting it
suppresses is a member of its canon's duplicate closure, and the canon's search document
SHALL carry that member's `countries`, `regions` and `cities` — see the canonical-job
geography union in `ingest-content-dedup`.

This is the pass's primary population, not an edge case: its similarity threshold was
calibrated on per-city variants of one role, so most rows it suppresses differ from their
canon exactly by location.

#### Scenario: The city of a suppressed posting stays searchable

- **WHEN** the fuzzy pass suppresses a posting open in a city the canon is not open in
- **THEN** a search filtered by that city still returns the canon
