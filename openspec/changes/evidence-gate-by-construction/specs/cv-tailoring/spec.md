## ADDED Requirements

### Requirement: The evidence gate is a construction-time dependency of the editor

The component that answers "may this claim be published?" SHALL be supplied when the CV
editor is created, and SHALL NOT be attachable afterwards. No assembly order, and no
combination of features built or not built, may produce an editor that admits an agent's
uncited claim.

The system SHALL NOT carry more than one place that answers the evidence question by
returning "permitted" for a missing dependency. Where the gate's own collaborator is
absent, that is a construction error, not a silent permission.

An editor built with no gate remains legitimate for candidate-authored editing only —
the candidate writing about their own career is the source the bank exists to record, and
the rule already exempts them — but it SHALL NOT be reachable from any agent write path.

#### Scenario: The agent write path cannot be built without a gate

- **WHEN** the HTTP surface is assembled
- **THEN** the editor serving `PATCH /me/cvs/:id` carries the evidence gate as a value it
  was constructed with, so an API-key caller editing as the agent is checked regardless of
  which other features the assembly built

#### Scenario: The gate does not depend on another feature's constructor

- **WHEN** the CV handlers are built in an assembly that does not build the assistant
- **THEN** the editor still refuses an agent's uncited claim, because the gate was never
  the assistant's to attach

#### Scenario: A missing bank is not a permission

- **WHEN** the gate is asked whether a claim is publishable and its backing bank is absent
- **THEN** the answer is not "permitted" — the absence is a wiring error the assembly
  cannot express, rather than a silent pass
