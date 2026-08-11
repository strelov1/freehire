## ADDED Requirements

### Requirement: The agent can read the achievements it is named

The agent SHALL be able to retrieve the full content of specific achievements by their
identifiers, naming several in one request. What comes back SHALL carry the same fields a
search returns — the claim, the situation it happened in, its metrics and skills, the role
it belongs to, and whether it may be written into a CV as it stands — so an achievement
reads the same however the agent obtained it.

This closes the gap that every id-bearing surface opens. The interviewer's opening message
names selected achievements by id, the profile summary reports near-duplicate clusters as
ids without their text, and a merge answers with the id it kept. Each of these hands the
agent an identifier, and an agent that cannot resolve one is reduced to guessing at
achievements the candidate is looking straight at.

The number of achievements one request may read SHALL be bounded, for the same reason a
search's results are bounded: a tool result is replayed into the model's context on every
later turn, so an unbounded read would consume the conversation's window.

An identifier that names nothing the caller owns SHALL be reported back as unresolved
alongside the achievements that did resolve, rather than failing the whole request. A
partial answer is the useful one, and it tells the agent exactly which of its assumptions
was wrong. An achievement belonging to another account SHALL be indistinguishable from one
that does not exist.

#### Scenario: Achievements named in the opening message resolve

- **WHEN** the interviewer opens on a set of achievements the candidate selected, and the
  agent reads the identifiers its opening message names
- **THEN** it receives those achievements' claims, context, metrics and skills, and its
  first reply discusses those achievements rather than any others

#### Scenario: A duplicate cluster can be read

- **WHEN** the profile summary reports a cluster of near-duplicate achievements as
  identifiers, and the agent reads them
- **THEN** it receives the text of each one and can tell the candidate what the
  near-duplicates actually say

#### Scenario: An unresolvable identifier does not fail the read

- **WHEN** the agent reads several identifiers of which one names no achievement of the
  caller's
- **THEN** the achievements that resolved are returned, and the one that did not is
  reported as unresolved

#### Scenario: Another account's achievement is not readable

- **WHEN** the agent reads an identifier belonging to a different account
- **THEN** it is reported as unresolved, with nothing of that achievement disclosed

### Requirement: An achievement is read before it is merged or refined

The interviewer SHALL direct the agent to read the achievements it has been named before
proposing to merge them or writing a refinement to one.

The tool alone does not produce this. Merging is decided entirely by the system — the
richer achievement is kept, metrics and skills are unioned — so an agent that never reads
the pair can still merge it, and cannot make the one judgement a merge requires: whether
the two describe the same work.

Refinement carries a narrower hazard that is just as destructive. A refinement leaves
fields it does not mention alone, but the fields it does mention are replaced whole: an
achievement's metrics and skills are lists that are set, not appended to. An agent adding a
newly learned metric to an achievement it has not read therefore replaces every metric
already recorded with the one it just heard.

#### Scenario: A proposed merge shows the candidate what it read

- **WHEN** the candidate selects two near-duplicate achievements and the agent proposes
  merging them
- **THEN** it first states what the two achievements say, so the candidate can confirm or
  deny that they are the same work, rather than asking for a merge it has not examined

#### Scenario: Adding a metric keeps the metrics already recorded

- **WHEN** the candidate gives a new number for an achievement that already carries two
  metrics
- **THEN** the achievement ends up carrying all three, rather than only the newest

#### Scenario: A refinement leaves untouched fields alone

- **WHEN** a refinement sets only an achievement's claim
- **THEN** its context, metrics and skills are unchanged

### Requirement: The in-place interviewer does not narrow the bank

Opening the interviewer in place SHALL NOT reduce the width the bank had while it was
closed. The conversation is about the achievements on screen, so a layout that pays for the
conversation by squeezing them defeats the reason for putting the two side by side.

On a viewport wide enough to show both, the page MAY move to make room for the panel, but
the bank SHALL keep its width. On a viewport too narrow for both, the existing behaviour
stands: the panel covers the bank and offers a way back.

#### Scenario: The bank keeps its width when the conversation opens

- **WHEN** the candidate opens the interviewer in place on a wide viewport
- **THEN** the bank's achievements are laid out no narrower than they were before it
  opened

#### Scenario: A narrow viewport still covers rather than splits

- **WHEN** the interviewer is opened in place on a viewport too narrow to show both
- **THEN** the panel covers the bank and a visible control returns to it, rather than the
  two sharing the width
