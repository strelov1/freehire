## ADDED Requirements

### Requirement: The derive backfill completes despite corrupted rows

The whole-catalogue derive backfill SHALL read jobs through the resilient full-scan
helper, so a row that cannot be read due to data corruption is skipped and logged rather
than ending the run. Every readable row after the corrupted one SHALL still be re-derived
in the same pass.

Because the backfill records no resume point, an aborting scan is not merely a failed run:
it re-fails at the same id on every subsequent run, so every deterministic column past that
row stays stale indefinitely. Skipping is what makes the pass finishable at all.

The scan SHALL treat a keyset cursor that did not advance as its exhaustion signal, and
SHALL NOT treat a page shorter than the batch size as the end of the table. The degrade
path returns a short page whenever it skips a damaged row, so the shorter-than-batch test
would end the scan at the first corrupted row and report a complete pass.

#### Scenario: A corrupted row does not end the backfill

- **WHEN** the derive backfill's scan meets a row that fails to read with a data-corruption
  error
- **THEN** that row is skipped and logged, and the rows after it are still scanned and
  re-derived in the same run

#### Scenario: A short page from the degrade path does not end the scan

- **WHEN** a page returns fewer rows than the batch size because a corrupted row was
  skipped
- **THEN** the scan continues from the advanced cursor rather than treating the short page
  as the end of the table

#### Scenario: A non-corruption read failure still ends the run

- **WHEN** the scan's read fails with an error that is not data corruption
- **THEN** the run fails with that error, exactly as before — the resilience is narrow to
  corruption and hides nothing else
