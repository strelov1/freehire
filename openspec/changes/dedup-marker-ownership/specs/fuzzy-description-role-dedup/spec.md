## MODIFIED Requirements

### Requirement: The fuzzy pass runs after and never overrides the exact pass

The fuzzy-description pass SHALL run AFTER the exact role-cluster recompute and operate only
over its remaining open canonical rows, so it merges what byte-exact matching left split and
never re-splits or contradicts a deterministic collapse. It SHALL write only its own marker
column, `duplicate_of_fuzzy`, and SHALL be idempotent across a full refresh cycle — the
recompute no longer reverses it, because the recompute no longer reaches it.

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
