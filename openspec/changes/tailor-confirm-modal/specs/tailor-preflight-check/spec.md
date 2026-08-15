## Purpose

Gives a candidate an explicit, informed checkpoint — built entirely from data the system
already computes deterministically — before they commit to tailoring a CV for a specific job,
so a tailoring credit is never spent on a role their own profile already shows an obvious gap
against without their knowledge.

## ADDED Requirements

### Requirement: Tailoring is confirmed before it starts

The system SHALL show a confirmation prompt every time a candidate initiates CV tailoring for a
job from a "Tailor CV" control (extension side panel, application-tracking drawer, or job-page
sidebar), before navigating to the tailoring workspace. The prompt SHALL be shown unconditionally
— including when the job is a strong match — rather than only when a gap exists. Declining SHALL
NOT navigate to the tailoring workspace and MUST NOT be treated as a tailoring credit spend.

#### Scenario: Confirming proceeds to tailoring

- **WHEN** a signed-in candidate clicks "Tailor CV" for a job and confirms the prompt
- **THEN** the system proceeds to the tailoring workspace for that job, exactly as it would have
  without the prompt

#### Scenario: Declining does not start tailoring

- **WHEN** a signed-in candidate clicks "Tailor CV" for a job and dismisses or cancels the prompt
- **THEN** the system does not navigate to the tailoring workspace and does not spend a
  tailoring credit

#### Scenario: A perfect match still shows the prompt

- **WHEN** a candidate whose profile satisfies every skill and requirement of a job clicks
  "Tailor CV"
- **THEN** the system still shows the confirmation prompt, presenting an all-clear summary
  instead of omitting the prompt

#### Scenario: Revisiting an existing analysis is not gated

- **WHEN** a candidate who has already tailored or analysed a job clicks a control that reopens
  that existing analysis (rather than a fresh "Tailor CV" action)
- **THEN** the system navigates directly, without showing the confirmation prompt

### Requirement: The prompt presents the existing deterministic fit check

The confirmation prompt SHALL present, for the job being tailored, the candidate's skill
coverage (which required skills are met and which are missing) and the outcome of every
applicable hard-constraint requirement (for example location, work mode, and work
authorization), using only data the system already computes without a language model. The
prompt MUST NOT trigger a language-model call and MUST NOT spend a tailoring credit merely by
being shown.

#### Scenario: Missing skills and unmet requirements are shown together

- **WHEN** the confirmation prompt opens for a job the candidate is missing required skills for
  and that carries at least one unmet hard-constraint requirement
- **THEN** the prompt lists the missing skills and the unmet requirements, and offers to proceed
  anyway rather than blocking the candidate from tailoring

#### Scenario: A fully-met job shows an all-clear summary

- **WHEN** the confirmation prompt opens for a job where every required skill is covered and
  every hard-constraint requirement is met
- **THEN** the prompt states that the candidate matches the job's requirements instead of
  listing any gap

#### Scenario: The check costs no AI credit

- **WHEN** the confirmation prompt is shown, regardless of outcome
- **THEN** no language-model call is made and no tailoring credit is debited as a result of
  showing it
