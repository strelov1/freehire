## ADDED Requirements

### Requirement: The agent's tailoring context carries the bank's answer per requirement

The tailoring agent's context tool SHALL attach, to every requirement it reports, the evidence the
candidate's experience bank already holds for it — each piece named by the id a CV edit must cite,
so the agent knows before it acts which requirements it can evidence and which it must ask about.
Retrieval MUST be the same scoring the search tool uses, and MUST NOT call a model: it is a scan
over the caller's own atoms, so attaching it costs a round nobody has to spend.

A requirement the bank has nothing for MUST say so explicitly rather than omit the field, because
"no evidence" is the answer that decides whether to ask the candidate, and an absent field reads
as "not looked at".

#### Scenario: A requirement the bank can evidence arrives with its evidence

- **WHEN** the agent reads the tailoring context for a vacancy whose requirement the bank holds evidence for
- **THEN** that requirement carries the evidence's id and claim, ready to be cited in an edit

#### Scenario: A requirement the bank cannot evidence says so

- **WHEN** a reported requirement has nothing scoring against it in the bank
- **THEN** it is reported with an empty evidence list rather than with the field left out

#### Scenario: Reading the context calls no model

- **WHEN** the agent reads the tailoring context
- **THEN** the evidence attached to each requirement comes from local scoring, with no LLM call

### Requirement: The agent's context carries what it can act on, not the narrative

The tailoring context served to the AGENT SHALL carry the vacancy, the verdict and score, and the
requirements with their evidence — and SHALL NOT carry the per-dimension comments, strengths, gaps
or recommendation the endpoint serves the page. None of them is something a CV edit can be made
from, all of them are on the candidate's screen already, and on a measured session they were 3 KB
of an 11.4 KB result the agent had to carry for the rest of the turn.

#### Scenario: The narrative sections stay with the page

- **WHEN** the agent reads the tailoring context
- **THEN** the result carries the vacancy, verdict, score and the evidenced requirements, and none of the dimension comments, strengths, gaps or recommendation

#### Scenario: The endpoint is unchanged

- **WHEN** a client reads the tailoring context over HTTP
- **THEN** it receives the full projection it received before, narrative sections included

### Requirement: The agent reads the posting as text, not as markup

The tailoring context served to the agent SHALL render the vacancy description from stored HTML to
plain text, and SHALL bound its length, exactly as the vacancy-reading tool already does. The
posting is the least trusted text in the conversation and the largest; sending it as markup spends
the turn's context on tags and widens what the model is asked to interpret.

#### Scenario: The description reaches the model without markup

- **WHEN** the agent reads the tailoring context for a vacancy whose stored description is HTML
- **THEN** the description it receives carries the posting's words and none of its tags

#### Scenario: A very long posting is bounded

- **WHEN** the stored description is longer than the context allows
- **THEN** it is truncated to the bound rather than sent whole
