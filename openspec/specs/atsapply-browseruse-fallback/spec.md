# atsapply-browseruse-fallback Specification

## Purpose

Lets an auto-apply attempt for Ashby or Workable execute and submit through a
browser-driving agent when the deterministic pipeline has already fully answered every
required question, instead of parking solely because no hand-written browser-fill path
exists for that platform.

## Requirements

### Requirement: Execution is offered only for a fully-resolved plan on an eligible provider

The system SHALL execute an application through the browser-use backend only when the
deterministic pipeline reports the attempt's `Plan` as fully resolved (every required
question answered) AND the posting's provider is Ashby or Workable AND the backend is
enabled. Every other case — an unresolved plan, any other provider, or the backend
disabled — SHALL be unaffected and behave exactly as before this capability existed.

#### Scenario: A fully-resolved Ashby plan executes through the backend

- **WHEN** a queued Ashby attempt's plan has no unmapped required questions and the
  backend is enabled
- **THEN** the attempt is executed through the browser-use backend rather than parked

#### Scenario: An unresolved plan still parks regardless of provider

- **WHEN** a queued Ashby or Workable attempt's plan still has at least one unmapped
  required question
- **THEN** the attempt is recorded as unresolved exactly as it is today, and the
  browser-use backend is never invoked

#### Scenario: A Greenhouse attempt is never routed to this backend

- **WHEN** a queued attempt's provider is Greenhouse, whether or not its plan is fully
  resolved
- **THEN** the browser-use backend is never invoked for it

#### Scenario: A captcha-protected or unscannable-form attempt is never routed to this backend

- **WHEN** a queued attempt would otherwise be parked for a challenge-protected form or an
  unrecognized form layout
- **THEN** it is parked exactly as today, and the browser-use backend is never invoked

### Requirement: The backend never chooses what to answer

The system SHALL pass the browser-use backend only field values the deterministic
pipeline already resolved, and SHALL instruct it not to act on any field outside that
resolved set. The backend SHALL NOT be the source of any answer's content — every value it
types is one this system already decided before the backend ever runs.

#### Scenario: A field outside the resolved plan is left untouched

- **WHEN** an eligible posting's live form carries a field the resolved plan does not
  cover
- **THEN** the executed attempt does not act on that field

### Requirement: Submission is confirmed only from an explicit, structured signal

The system SHALL require the backend's report of an execution to end in one of exactly
three explicit outcomes — confirmed (naming the confirmation it observed), unconfirmed, or
parked (naming a reason) — and SHALL treat a report that supplies none of these the same
as unconfirmed. The system SHALL NOT infer a successful submission from unstructured
narrative text.

#### Scenario: A confirmed submission is recorded as applied

- **WHEN** the backend's report ends with an explicit confirmation naming what it observed
- **THEN** the attempt is recorded as successfully submitted

#### Scenario: A report with no explicit outcome is treated as unconfirmed

- **WHEN** the backend's report does not end in one of the three explicit outcomes
- **THEN** the attempt is treated as unconfirmed, exactly as an unconfirmed chromedp
  submission is today — dead-lettered rather than retried through the ordinary
  transient-failure path

### Requirement: Spend is bounded before and during every execution

The system SHALL cap the cost of a single execution and SHALL refuse to start a new
execution once a configured daily spend threshold is reached. The daily threshold SHALL
support a shadow mode that only logs what it would have refused, before it is switched to
actually refusing.

#### Scenario: A single execution cannot exceed its cost cap

- **WHEN** an execution's provider-reported cost would exceed the configured per-run cap
- **THEN** the execution is stopped rather than allowed to keep spending

#### Scenario: The daily threshold logs before it enforces

- **WHEN** the daily spend threshold is in shadow mode and would have been exceeded
- **THEN** the system logs what it would have refused and still allows the execution

#### Scenario: The daily threshold refuses once enforced

- **WHEN** the daily spend threshold is enforced and has been reached
- **THEN** a new execution is refused and the attempt is not sent to the backend
