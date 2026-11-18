# cv-tailoring-agent-context Specification

## Purpose
What the tailoring agent is given to read: the bank's answer per requirement, what it can act on, the posting as text, and never the CV contact block. Split out of cv-tailoring.

## Requirements

### Requirement: The tailoring agent never receives the CV contact block

The CV contact block MUST NOT reach any reader that puts the document in front of a model. That
covers both agent-facing read paths — the HTTP read authenticated with the short-lived tailoring
key, and the in-process assistant's `cv_get` tool — each of which SHALL omit the `Header` contact
fields (`full_name`, `email`, `phone`, and the personal links list) from the document it returns,
and SHALL reject any patch that targets those fields.

The omission SHALL be performed by one shared redaction in the service path, applied by every
agent-facing reader, rather than by a per-transport check in a handler. A guard keyed on how the
request arrived (holding an API key, say) does not survive a new agent surface that arrives over a
different transport or none at all, and the requirement is about who is reading, not how they got
there. A new surface must not be able to inherit the CV-reading capability without inheriting the
redaction.

The stored contact values are unchanged and appear in the rendered output (served on the owner's
own cookie-authenticated read and the PDF), so the finished CV is complete while the agent's model
never sees the identifiers. The candidate's own cookie-authenticated reads are unaffected.

#### Scenario: Agent read omits the contact block

- **WHEN** the tailoring key is used to read the CV document
- **THEN** the response document carries the body (experience, summary, skills, …) but no `full_name`, `email`, or `phone`

#### Scenario: The in-process tool read omits the contact block

- **WHEN** the assistant's `cv_get` tool reads the CV document during a tailoring session, holding no API key
- **THEN** the tool result carries the body but no `full_name`, `email`, `phone`, or personal links

#### Scenario: Agent cannot patch a contact field

- **WHEN** the tailoring key is used to patch `full_name`, `email`, or `phone`
- **THEN** the patch is rejected and the stored contact value is unchanged

#### Scenario: The owner still sees and renders full contacts

- **WHEN** the owner reads the CV over their cookie session, or the CV is rendered to PDF
- **THEN** the real contact block is present

#### Scenario: The tailored body carries no contact identifier back

- **WHEN** the agent patches the CV body during tailoring
- **THEN** no contact identifier is introduced into a body field (the agent never held one)

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
