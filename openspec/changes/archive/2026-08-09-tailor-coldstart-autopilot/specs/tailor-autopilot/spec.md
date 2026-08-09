## ADDED Requirements

### Requirement: An autopilot run satisfies its own analysis precondition

The system SHALL, when an autopilot run starts on a session whose vacancy has no cached fit
analysis yet, compute that analysis synchronously as part of the SAME request before starting the
turn, then proceed exactly as it does when a cached analysis already exists. This runs on the
request's own context (no detached background job), so the run's requirements source
(`cv_context`, sourced from the cached analysis) is always populated by the time the agent's first
tool call reads it. When a cached analysis already exists, this step is a no-op — today's behavior,
unchanged.

#### Scenario: A run with no cached analysis computes one first

- **WHEN** an autopilot run starts on a session whose vacancy has no cached fit analysis
- **THEN** the analysis is computed and cached before the turn's first tool call, and the run then
  proceeds and streams exactly as a run against an already-cached analysis does

#### Scenario: An existing cached analysis is reused, not recomputed

- **WHEN** an autopilot run starts on a session whose vacancy already has a cached analysis
- **THEN** no new analysis is computed and the run proceeds directly
