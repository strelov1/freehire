## ADDED Requirements

### Requirement: A user is on exactly one plan, read from one column

The system SHALL resolve every user to exactly one plan — `free` or `pro` — from
`users.pro_until` alone: a timestamp in the future means `pro`, and NULL or a past
timestamp means `free`. The resolution SHALL NOT call any external service, so that a
billing provider being slow or unreachable can never delay or fail a metered action.

#### Scenario: No subscription record

- **WHEN** a user whose `pro_until` is NULL performs a metered action
- **THEN** the action is evaluated against the free plan's allowances

#### Scenario: Subscription in force

- **WHEN** a user whose `pro_until` is later than now performs a metered action
- **THEN** the action is evaluated against the pro plan's allowances

#### Scenario: Subscription lapsed

- **WHEN** a user whose `pro_until` is earlier than now performs a metered action
- **THEN** the action is evaluated against the free plan's allowances, and nothing the
  user previously created — CVs, analyses, transcripts, tracked applications — is
  withdrawn or hidden

#### Scenario: Plan resolution touches no network

- **WHEN** the plan of a user is resolved on the request path
- **THEN** the answer is derived from stored state only, and no call is made to a payment
  or entitlement provider

### Requirement: Every plan offers every feature

The system SHALL make every metered feature reachable on every plan. A plan SHALL differ
only in how much of a feature it allows per day, never in whether the feature exists. No
metered feature SHALL be hidden, disabled or refused outright on the free plan.

#### Scenario: A free user reaches a feature the pro plan also has

- **WHEN** a free-plan user with allowance remaining invokes any metered feature
- **THEN** the feature runs, and its behaviour and output are the same as they would be
  for a pro-plan user

#### Scenario: Refusal names exhaustion, not entitlement

- **WHEN** a free-plan user is refused a metered action
- **THEN** the refusal states that today's allowance is spent and when it resets, and
  never states that the feature is unavailable on their plan

### Requirement: Allowances are counted per feature, per calendar day

The system SHALL meter each feature against its own daily allowance, keyed by the UTC
calendar day. An allowance SHALL reset at the start of each UTC day, and unused allowance
SHALL NOT carry into the next day. Exhausting one feature's allowance SHALL NOT reduce
any other feature's allowance.

#### Scenario: Day rolls over

- **WHEN** a user who exhausted a feature's allowance yesterday invokes it after the UTC
  day has changed
- **THEN** the action is allowed, and the allowance reads as freshly full

#### Scenario: Unused allowance does not bank

- **WHEN** a user consumes nothing on one day and invokes a feature the next day
- **THEN** the allowance available is that of a single day, not the sum of both

#### Scenario: Features do not share a counter

- **WHEN** a user exhausts one feature's daily allowance
- **THEN** every other metered feature remains available at its own untouched allowance

#### Scenario: Reset happens without a scheduled job

- **WHEN** the first metered action of a new day is evaluated for a user whose stored
  consumption is from an earlier day
- **THEN** the stale consumption is disregarded and the new day's allowance applies,
  without any background worker having run

### Requirement: The free plan's daily allowances

The system SHALL apply these allowances to a free-plan user per UTC day: **2** tailoring
sessions, **3** job fit analyses, **10** assistant messages across the chat and profile
presets combined, and **10** dictation transcriptions. These values SHALL be
configuration, not literals in call sites, so that they can be tuned without touching the
features they bound.

#### Scenario: Third tailoring session in a day

- **WHEN** a free-plan user who has started two tailoring sessions today starts a third
- **THEN** the system refuses with 402 and no session, CV or model call is created

#### Scenario: Fourth fit analysis in a day

- **WHEN** a free-plan user who has run three fit analyses today requests a fourth for a
  job they have not analysed before
- **THEN** the system refuses with 402 before the prompt chain runs

#### Scenario: Chat and profile presets share one allowance

- **WHEN** a free-plan user has sent ten assistant messages today across any mix of the
  chat and profile presets
- **THEN** the eleventh message in either preset is refused with 402

### Requirement: A tailoring session carries a turn ceiling

The system SHALL bound the number of assistant turns inside a single tailoring session,
in addition to bounding how many sessions a day may be started. The ceiling SHALL be
**15** turns per session on the free plan. A turn beyond the ceiling SHALL be refused
with 402 naming the session, and the user SHALL be able to spend another tailoring
session's allowance to continue in the same session.

#### Scenario: Turn ceiling reached inside an allowed session

- **WHEN** a free-plan user who started a tailoring session within today's allowance
  sends the sixteenth turn in that session
- **THEN** the system refuses with 402, the session and its CV are left intact, and the
  refusal offers to continue by consuming another session's allowance

#### Scenario: Continuing past the ceiling consumes allowance

- **WHEN** a user with tailoring allowance remaining elects to continue a session that
  reached its turn ceiling
- **THEN** one tailoring session allowance is consumed and the session's ceiling is
  extended by one ceiling's worth of turns

#### Scenario: Continuing a session that predates the metering

- **WHEN** a session created before this metering shipped — one holding no consumption at
  all, and therefore running under the one ceiling it is granted implicitly — reaches that
  ceiling and its owner elects to continue
- **THEN** one tailoring session allowance is consumed AND the session's ceiling is
  extended by one ceiling's worth of turns, exactly as for a session that was charged when
  it started; the allowance MUST NOT be consumed for a ceiling the session already had

#### Scenario: Session count alone does not bound depth

- **WHEN** a user opens two tailoring sessions in a day and runs many turns in each
- **THEN** each session is independently stopped at its own turn ceiling, so the total
  turns available in a day is bounded by both limits together

### Requirement: The pro plan is bounded only by a fair-use guard

The system SHALL apply no user-facing daily ceiling to a pro-plan user. It SHALL still
enforce a fair-use guard per feature, set far above observed human behaviour, whose only
purpose is to stop an automated caller from draining the gateway on one subscription.
Crossing the fair-use guard SHALL be reported to operators; it SHALL NOT be presented to
the user as a plan limit.

#### Scenario: Ordinary pro usage is never stopped

- **WHEN** a pro-plan user performs any metered feature at a volume consistent with human
  use
- **THEN** no refusal is issued and no allowance is displayed as running out

#### Scenario: Automated caller on a pro plan

- **WHEN** a pro-plan account exceeds a feature's fair-use guard within a day
- **THEN** further calls to that feature are refused for the rest of the day and the
  event is recorded for operators

### Requirement: Allowance is reserved before the work and released when it produces nothing

The system SHALL consume a feature's allowance BEFORE the model work begins, and SHALL
return it when the work produces no result the user can use. Consumption SHALL be
idempotent by `(user, feature, ref)`, so that a retry, a reconnect or a recompute of work
already paid for consumes nothing further. Releasing an allowance that was never consumed
SHALL be a no-op, so that every failure path may call it without first establishing
whether it owes one.

#### Scenario: Refusal happens before any model call

- **WHEN** a user without remaining allowance invokes a metered feature
- **THEN** the request is refused before any language-model call is made and before any
  row is written

#### Scenario: Work fails after the allowance was taken

- **WHEN** the model work for a metered action fails and produces nothing the user can
  use
- **THEN** the allowance is returned, and the same action may be attempted again and is
  charged exactly once when it succeeds

#### Scenario: Retrying an action already charged

- **WHEN** the same `(user, feature, ref)` is submitted a second time after a successful
  first consumption
- **THEN** no further allowance is consumed and the action proceeds

#### Scenario: Concurrent first actions of a new user

- **WHEN** two requests for the same user and feature are evaluated at the same moment
  and only one allowance remains
- **THEN** exactly one succeeds and the other is refused, and the recorded consumption
  never exceeds the allowance

### Requirement: The ledger remains the append-only record of consumption

The system SHALL record every allowance consumption, release and grant as an append-only
ledger entry carrying the user, the feature, the day, the reference and the delta. Any
materialised per-day counter SHALL be derivable from the ledger, so that a disagreement
between the two is resolvable in favour of the ledger.

#### Scenario: Consumption is recoverable from the ledger

- **WHEN** the per-day counters are rebuilt from the ledger for a given user and day
- **THEN** the result matches what the metering path enforced on that day

#### Scenario: A release is recorded, not erased

- **WHEN** an allowance is returned after failed work
- **THEN** the ledger shows both the consumption and its release rather than showing
  neither

### Requirement: Enforcement is a per-feature switch that ships off

The system SHALL carry a per-feature enforcement switch, defaulting to OFF, and SHALL NOT
turn a caller away for a feature whose switch is off. With the switch off a spent allowance
SHALL still be counted and reported, so the shadow run can answer how many callers a ceiling
would have stopped against live traffic before anybody is refused on a number nobody has
verified. The switch SHALL be settable per feature without a deploy.

The enforcement state SHALL travel with every allowance the API returns, and NO surface —
server-side pre-check, SPA or extension — may hide, disable or refuse an action on
"consumed has reached allowed" alone. The fair-use guard is the one exception: it protects
the gateway rather than a price, and is not subject to the switch.

#### Scenario: A spent allowance under a switched-off feature still runs

- **WHEN** a free-plan user has consumed today's whole allowance for a feature whose
  enforcement is off, and takes that action again
- **THEN** the action runs, the consumption is recorded, and the record marks that a live
  ceiling would have refused it

#### Scenario: A client cannot pre-block what the server would allow

- **WHEN** a surface renders an action whose allowance is consumed but whose enforcement is
  off
- **THEN** the action remains available and is not presented as refused — a client-side
  block would both refuse a caller the server would serve and withhold from the shadow
  measurement the very requests it exists to count

### Requirement: A refusal is HTTP 402 and says what to do next

The system SHALL refuse an exhausted metered action with HTTP **402**, and the body SHALL
name the feature that is exhausted, the instant the allowance resets, and where the user
can upgrade. A streaming endpoint SHALL issue the 402 as a real HTTP status before the
stream opens, not as an error frame inside an already-successful stream.

#### Scenario: Refusal body is actionable

- **WHEN** a metered action is refused for want of allowance
- **THEN** the response is 402 and its body identifies the feature, the reset instant and
  the upgrade destination

#### Scenario: Streamed endpoint refuses before opening the stream

- **WHEN** a user without remaining allowance opens a streaming metered endpoint
- **THEN** the response is a 402 HTTP status and no event stream is opened

### Requirement: The user can see today's usage

The system SHALL expose the caller's plan, and for each metered feature the amount
consumed today, the amount allowed, and the reset instant. The surface SHALL express this
as usage against a limit — never as a balance of a currency — and the word "credits"
SHALL NOT appear in it.

#### Scenario: Free user reads their usage

- **WHEN** a free-plan user requests their usage
- **THEN** the response names their plan and, per feature, what they have used today,
  what they are allowed, and when it resets

#### Scenario: Pro user reads their usage

- **WHEN** a pro-plan user requests their usage
- **THEN** the response names their plan and reports each feature as unlimited rather
  than reporting a remaining number
