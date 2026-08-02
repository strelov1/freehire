# apply-form-display Specification

## Purpose
TBD - created by archiving change show-apply-questions. Update Purpose after archive.
## Requirements
### Requirement: A posting's stored application form is served for display

The system SHALL serve, for one posting identified by its public slug, the
application form stored for it, in a shape meant for reading rather than for
submitting. Where no form is stored the system SHALL answer that none exists rather
than an empty form, because "this employer asks nothing" and "we could not read
this platform" are different statements and only one of them is true.

The served shape SHALL carry the provider the form was read from, so a reader can
say where the answer came from.

#### Scenario: A posting with a stored form

- **WHEN** the form of a posting that has one is requested
- **THEN** the response carries that posting's questions and the provider they were
  read from

#### Scenario: A posting with no stored form

- **WHEN** the form of a posting that has none is requested
- **THEN** the response says no form exists, and is distinguishable from a form
  with no questions

#### Scenario: A posting that does not exist

- **WHEN** a form is requested for a slug no posting carries
- **THEN** the response says so, in the same way the posting's own endpoint does

### Requirement: The display projection shows the employer's questions and nothing else

The served form SHALL carry, for each question the employer authored: the question
text exactly as the platform published it, whether an answer is required, and the
kind of answer expected.

The projection SHALL exclude:

- The controls every application demands — the candidate's name, contact details
  and CV — presented instead as one entry, so their presence is stated once rather
  than padding the list with what everyone expects.
- Every control the platform itself classifies as an equal-opportunity or
  demographic survey question. Those are not the employer's questions; they are a
  mandated survey the platform serves in its own block, always optional and
  near-identical everywhere, and listing them would bury the questions a candidate
  actually needs to prepare for.
- Every control that is not a question at all: one the platform fills itself and the
  candidate never sees, and any block of text an employer placed in the middle of
  the form.

#### Scenario: An employer's question is shown whole

- **WHEN** a stored form carries an employer's question
- **THEN** the projection shows its text as published, whether it is required, and
  the kind of answer it expects

#### Scenario: The standard fields are stated once

- **WHEN** a stored form carries the name, email, phone and CV controls
- **THEN** the projection presents them as a single entry rather than one per control

#### Scenario: The survey questions are dropped

- **WHEN** a stored form carries controls the platform marked as demographic
- **THEN** none of them appear in the projection, and the employer's own questions
  are unaffected

#### Scenario: Non-questions are dropped

- **WHEN** a stored form carries a hidden control or a block of explanatory text
- **THEN** neither appears in the projection

#### Scenario: A form of nothing but standard fields

- **WHEN** a stored form carries no employer questions at all
- **THEN** the projection is served and says so — a form that asks only for a CV is
  itself worth knowing

### Requirement: A question's answer kind is named, and never guessed

The projection SHALL name the kind of answer each question expects, drawn from a
fixed vocabulary, so that a question answerable in a word is distinguishable from
one demanding an essay — which is the difference that decides whether applying
costs a minute or an evening.

Where the stored form's control kind is one the capture could not normalize, the
projection SHALL name no kind at all rather than the nearest one. The question is
still shown; only the hint about its cost is withheld.

#### Scenario: A written answer is distinguishable from a one-liner

- **WHEN** the projection carries a long-form question and a single-line question
- **THEN** the long-form one is named as expecting a written answer and the other
  is not

#### Scenario: An unrecognized kind yields no name

- **WHEN** a question's stored kind is one the capture left unnormalized
- **THEN** the question appears with no answer kind named

### Requirement: The job page shows the questions, and its absence costs nothing

The job detail page SHALL request the posting's form alongside the other
non-essential requests it already makes, and SHALL render the questions when one
comes back.

A failure or an absent form SHALL leave the rest of the page untouched: the block is
simply not rendered. The form is a discovery aid, and a discovery aid must never
be able to break the page it sits on.

#### Scenario: A posting whose form is known

- **WHEN** the page is rendered for a posting with a stored form
- **THEN** the questions appear on it

#### Scenario: A posting whose form is not known

- **WHEN** the page is rendered for a posting with no stored form
- **THEN** the page renders exactly as it does today, with no block and no error

#### Scenario: The request fails

- **WHEN** the form request fails for any reason
- **THEN** the page still renders, without the block

