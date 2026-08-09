## REMOVED Requirements

### Requirement: Tailoring requires an existing fit analysis

**Reason**: Cold start no longer gates on a pre-existing analysis. The bootstrap now always
succeeds (once a base CV can be seeded) and starts the analysis itself as part of the same cold
start, instead of requiring the caller to have produced one first.

**Migration**: See the new "Cold start assembles the CV without waiting on a pre-existing
analysis" requirement below. Callers that relied on the `409 "run the fit analysis first"` response
(the frontend's inline SSE-stream-and-retry flow added by the match/tailor merge) no longer receive
it; the bootstrap response itself now indicates when a cold-start run has just been started.

The system SHALL require a cached fit analysis for the (user, vacancy) pair before tailoring, and
MUST NOT recompute it. When no cached analysis exists, the bootstrap MUST fail with a 409 telling
the user to run the fit analysis first.

#### Scenario: Tailoring is refused when no analysis is cached

- **WHEN** a beta user requests tailoring for a vacancy they have never analyzed
- **THEN** the request fails with a 409 telling them to run the fit analysis first

#### Scenario: Bootstrap returns the cached analysis without recomputing

- **WHEN** a beta user with a cached analysis requests tailoring
- **THEN** the response carries that cached analysis and no LLM call is made

### Requirement: Tailoring is beta-gated and surfaced only after analysis

**Reason**: Split into a plain beta-gate requirement below. The "surfaced only after analysis"
clause described a `/match` fit-page CTA that no longer exists after the match/tailor merge — every
current entry point already links unconditionally to `/tailor/[slug]`, and this change removes the
last reason such a condition could exist (tailoring never again requires a pre-existing analysis).

**Migration**: See "Tailoring is beta-gated" below for the surviving beta-access rule.

The system SHALL gate every tailoring endpoint behind beta access (the union of the CV builder gate
and the agent's beta-tester flag), and the fit page SHALL surface the "tailor my CV" entry point
only when a cached, non-stale analysis exists for that user and vacancy.

#### Scenario: A non-beta user cannot reach tailoring

- **WHEN** a signed-in non-beta user calls the tailoring bootstrap
- **THEN** the request is refused by the beta gate

#### Scenario: The CTA is hidden without an analysis

- **WHEN** a beta user opens a fit page for a vacancy they have not analyzed
- **THEN** the "tailor my CV" entry point is not shown

## ADDED Requirements

### Requirement: Tailoring is beta-gated

The system SHALL gate every tailoring endpoint behind beta access — the union of the CV builder
gate and the agent's beta-tester flag.

#### Scenario: A non-beta user cannot reach tailoring

- **WHEN** a signed-in non-beta user calls the tailoring bootstrap
- **THEN** the request is refused by the beta gate

### Requirement: The bootstrap response flags a first-time cold start

The system SHALL NOT require a cached fit analysis to exist before starting tailoring. The
bootstrap response SHALL carry a boolean indicating whether this call just created a new tailored
CV for a (user, vacancy) pair that had none yet — reusing the same "just created" signal the
bootstrap already computes today (creation and last-update timestamps equal) — so the workspace can
tell a genuine cold start from re-opening an existing CV. Nothing about analysis or autopilot is
started by the bootstrap itself; the flag only signals the workspace to trigger the autopilot run
automatically (see `tailor-autopilot`'s cold-start requirements), instead of gating or sequencing
anything server-side at bootstrap time.

Repeating the bootstrap for the same (user, vacancy) pair MUST NOT report a cold start a second
time — the existing idempotency rule (same CV, same conversation, no second debit) extends to this
flag: it is true on at most one bootstrap call per (user, vacancy).

#### Scenario: A first-time bootstrap flags a cold start

- **WHEN** a beta user with a base CV requests tailoring for a vacancy they have never tailored
  before
- **THEN** the bootstrap response returns immediately with the new tailored CV, the session id, and
  the cold-start flag set to true, without waiting on any analysis or autopilot run

#### Scenario: Repeating the bootstrap does not flag a cold start again

- **WHEN** the bootstrap is requested again for a (user, vacancy) pair that already has a tailored CV
- **THEN** the existing CV and conversation are returned and the cold-start flag is false
