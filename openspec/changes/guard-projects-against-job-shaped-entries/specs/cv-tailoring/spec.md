## ADDED Requirements

### Requirement: A job-shaped project entry is refused for the agent

The system SHALL refuse an agent-authored operation that writes a `projects[i].name` carrying a
`<start year> - <end year or present>` date span, since `cv.Project` has no `company` or date
fields at all and a job filed there loses its tenure outright rather than merely sitting under the
wrong heading. The message MUST name `experience[]` as the place for a role with a start and end
date. This check MUST NOT run for a candidate's own edit — only `ActorAgent` batches are checked,
the same actor scope `requireEvidence` already uses.

The check MUST look only at a project's `name`, never its `bullets`: a bullet legitimately cites a
year as part of an achievement ("cut P95 latency in 2021") without the entry being misfiled.

Where a batch carries several operations, each MUST answer for itself, and one operation tripping
this refusal MUST reject the whole batch — the same all-or-nothing contract the evidence gate and
the bullet-cap guard already keep.

#### Scenario: A job-dated project name is refused

- **WHEN** the agent commits an operation setting `projects[0].name` to "Senior Engineer, Acme Corp (2020 - 2023)"
- **THEN** the commit is refused and the message names `experience[]` as the place for a role with a start and end date

#### Scenario: A project name with no date span is allowed

- **WHEN** the agent commits an operation setting a project's `name` to "Freelance Consulting, Acme Corp"
- **THEN** the commit proceeds — there is no date-range signal to refuse on

#### Scenario: A date mentioned in a bullet is allowed

- **WHEN** the agent commits an operation adding a bullet that reads "Cut P95 latency in half in 2021" to a project
- **THEN** the commit proceeds — only a project's `name` is checked, never its bullets

#### Scenario: The candidate's own edit is never refused by this check

- **WHEN** the candidate (not the agent) sets a project's `name` to "Senior Engineer, Acme Corp (2020 - 2023)" through their own CLI edit
- **THEN** the commit proceeds — this check only applies to `ActorAgent` batches

#### Scenario: One job-shaped project rejects the whole batch

- **WHEN** a batch writes two project names and one of them carries a job's date span
- **THEN** neither is applied
