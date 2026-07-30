## MODIFIED Requirements

### Requirement: The delta is owner-scoped and only defined for a tailored CV

The read SHALL be owner-scoped: a CV belonging to another account SHALL be indistinguishable from
one that does not exist. A CV with no defined comparison SHALL be refused as a conflict, not
answered with a fabricated baseline. Two distinct cases have no comparison, and the refusal SHALL
say which one it is rather than describing both as "not a tailored CV":

- the CV **is** the base CV — there is nothing to compare it against;
- the CV is a tailored copy whose **vacancy no longer exists** (pruned), so the keyword baseline
  both sides would be scored against is gone.

The baseline SHALL be the CV marked as the user's base. A vacancy-less tailored copy MUST NOT be
used as the baseline, however recently it was edited.

#### Scenario: Another account's CV is not found
- **WHEN** a caller reads the delta for a CV owned by a different account
- **THEN** the response is the same not-found as for a CV id that does not exist

#### Scenario: A base CV has no delta
- **WHEN** a caller reads the delta for the base CV
- **THEN** the response is a conflict saying it is the base, and no score is computed

#### Scenario: A tailored copy whose vacancy was pruned has no delta
- **WHEN** a caller reads the delta for a tailored copy whose vacancy row has been deleted
- **THEN** the response is a conflict saying the vacancy no longer exists, and no score is computed

#### Scenario: An orphan is never the baseline
- **WHEN** the user has a base CV and an orphaned tailored copy edited more recently, and a delta
  is read for a third, live tailored CV
- **THEN** the comparison is against the base CV, and the response names it as the baseline
