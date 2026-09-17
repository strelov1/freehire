## ADDED Requirements

### Requirement: Global application response rate

The company rollup SHALL also maintain one all-companies response rate, summed from the same
per-company observable and answered counts the per-company figure is built from, and SHALL serve
it under the same ten-application sample gate the per-company figure uses. Below the floor the
global figure SHALL be absent, not zero and not an estimate.

The sum SHALL include every company's observable/answered counts regardless of whether that
company individually clears its own ten-application gate — the per-company gate governs what is
safe to publish about one named employer, not what may contribute to an aggregate across all of
them.

#### Scenario: Global rate above the gate

- **WHEN** the sum of every company's observable applications is at least ten
- **THEN** a global response rate is served

#### Scenario: Global rate below the gate

- **WHEN** the sum of every company's observable applications is under ten
- **THEN** no global response rate is served

#### Scenario: Small companies still contribute to the global sum

- **WHEN** every individual company has fewer than ten observable applications, but their combined
  total is fifteen
- **THEN** the global response rate is served, computed over all fifteen

#### Scenario: Read from the same post-rebuild snapshot as the per-company figure

- **WHEN** `cmd/rollup-company` runs
- **THEN** the global response rate, read as a live sum over the per-company rows, reflects
  exactly the snapshot those rows were replaced into within one transaction — never a mix of the
  old and new rebuild
