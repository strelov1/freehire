## Why

The board-retirement report reads absence of a technical signal as evidence that a
board has none to give. For most of the catalogue there is no signal either way:
`is_tech` is tri-state, and `jobderive` leaves it NULL rather than coercing, "so the
unclassified mass stays measurable". The report collapsed that third state into
`false`, so a board nobody had classified was indistinguishable from one determined
to be non-technical.

Measured on prod against the first full run of the report: of 17841 listed boards it
named, 11023 — 62% — had no verdict on a single posting, against 10.6% among the
boards the same run kept. The bias is structural rather than incidental, because the
`is_tech IS TRUE` half of the evidence test needs a verdict to fire at all, so an
unclassified board can only escape the list through the weaker tagged-skill signal.

The consequence is not theoretical. `bamboohr/irishtitan` was listed while its only
posting was the placeholder "No Positions Open"; a genuine `Associate Digital Project
Manager` appeared hours later. `teamtailor/hypehype` — a game studio — was listed on
the strength of one "Open Application" entry. Retiring a board stops it being crawled,
and the company-scoped prune rules then become armed against its postings, so acting
on those rows removes live IT employers from the catalogue for no observation at all.

## What Changes

- `cmd/prune --boards` lists a board only when at least one of its postings carries an
  `is_tech` verdict and none of them came out technical. A board no posting of which
  has been classified is withheld.
- The report states how many boards it withheld and why, so a silently shorter list
  cannot read as "this is everything" — the same accounting the scan already gives for
  what its source gate turns down.

## Impact

- Affected specs: `catalog-pruning`
- Affected code: `cmd/prune/main.go` (`reportBoards`), `cmd/prune/boards_test.go`
- No schema change: `PruneCandidates` already returns the tri-state `is_tech`, and the
  third state was being discarded at the point of use.
- Operationally the report shrinks. That is the point: the withheld boards are not
  safe, they are unmeasured, and they return to the report as classification reaches
  them.
