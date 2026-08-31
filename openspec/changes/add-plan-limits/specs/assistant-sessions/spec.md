## ADDED Requirements

### Requirement: An assistant turn consumes a plan allowance

The system SHALL consume one assistant-message allowance for each turn a user starts in the
chat and profile presets, before the turn's first model call. The chat and profile presets
SHALL draw on ONE shared allowance, because they are the same conversation surface pointed
at different things and a per-preset allowance would only teach the user which name to use.

A turn is the unit charged, not the model call. One turn ran 7.1 model calls on average in
production, and the number is invisible to the person who asked the question — charging per
call would make two identical-looking requests cost differently, for reasons the user cannot
see or influence.

A turn that produces nothing the user can use SHALL return its allowance.

#### Scenario: Turn within allowance

- **WHEN** a user with assistant-message allowance remaining sends a message in the chat or
  profile preset
- **THEN** the turn runs and one allowance is consumed

#### Scenario: Turn beyond allowance

- **WHEN** a user who has spent today's assistant-message allowance sends another message
- **THEN** the system responds `402` before any model call, and the session and its
  transcript are unchanged

#### Scenario: Chat and profile draw on the same allowance

- **WHEN** a user consumes their assistant-message allowance entirely in the chat preset
- **THEN** the profile preset refuses the next message with `402`

#### Scenario: A failed turn returns its allowance

- **WHEN** a turn is charged and then fails at the model or transport before producing any
  answer
- **THEN** the allowance is returned and the user may retry

#### Scenario: Resuming an interrupted turn is not charged again

- **WHEN** a user resumes a turn that was already charged and interrupted mid-flight
- **THEN** the resumed turn consumes no further allowance

#### Scenario: Reading a transcript costs nothing

- **WHEN** a user with no allowance remaining opens or scrolls an existing session
- **THEN** the transcript is served in full and nothing is consumed

### Requirement: The tailor preset is metered by its own session rules

The system SHALL NOT charge a turn in the tailoring preset against the assistant-message
allowance. Tailoring is metered by its own two bounds — a daily session count and a
per-session turn ceiling — and charging it twice would make the daily assistant allowance
decide how deep one CV may be edited.

#### Scenario: A tailoring turn leaves the assistant allowance alone

- **WHEN** a user runs turns inside a tailoring session
- **THEN** their assistant-message allowance for the day is unchanged

#### Scenario: An exhausted assistant allowance does not block tailoring

- **WHEN** a user who has spent today's assistant-message allowance sends a turn inside a
  tailoring session that is within its own bounds
- **THEN** the turn runs
