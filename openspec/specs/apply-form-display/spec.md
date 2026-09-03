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

### Requirement: Workable's standard profile is told apart from the employer's questions

The display projection SHALL treat a Workable control as the employer's own question
only when the platform marks it as one, and SHALL treat every other control as part of
the standard profile every Workable application collects.

Workable marks an employer's question by prefixing its identifier, which is the
platform's own convention rather than an inference from the question's wording.

#### Scenario: An employer's question is shown

- **WHEN** a captured Workable form carries a control the platform marked as an
  employer question
- **THEN** the projection shows it among the questions

#### Scenario: The standard profile is collapsed

- **WHEN** a captured Workable form carries the name, email, phone, CV, education and
  experience controls
- **THEN** none of them appear as questions, and all appear once in the standard entry

### Requirement: The block states how much work the form is before it lists it

The job page SHALL state, above the questions, how many questions the form asks.
Where any of them demand a written answer, it SHALL state how many do.

The count of written answers SHALL be omitted where none are demanded, because a
stated zero reads as a measurement of something absent rather than as the absence
of the thing measured, and the reader is being told what the form will cost — not
what it will not.

These figures SHALL be derived from the questions the page was served, so that a
figure can never disagree with the list beneath it.

#### Scenario: A form demanding written answers

- **WHEN** the page renders a form whose questions include some demanding a
  written answer
- **THEN** the block states both the total number of questions and how many demand
  a written answer

#### Scenario: A form demanding none

- **WHEN** the page renders a form no question of which demands a written answer
- **THEN** the block states the total number of questions and says nothing about
  written answers

#### Scenario: A form of standard fields alone

- **WHEN** the page renders a form carrying no employer questions at all
- **THEN** the block states no count, and still shows what the application
  collects

### Requirement: Questions are grouped by what answering one costs

The job page SHALL group the served questions by the kind of answer each expects,
and SHALL present the groups cheapest first: the questions answerable in a line,
then those answered by choosing from what the employer offers, then those
demanding written answers, then those demanding a file.

Each group's heading SHALL name the kind of answer its questions expect and how
many there are, so that the kind is stated once for the group rather than once per
question.

A group no question falls into SHALL NOT be shown.

Where every question falls into a single group, that group's heading SHALL be
omitted only where the kind it names is already known to the reader without it:
where the summary above has already counted that kind, or where the kind is the
one nothing has ever named — the one-line answer everybody assumes. In every other
case the lone heading SHALL be shown, because the kind moved out of the questions'
own rows and into the heading, so suppressing it would not repeat a fact but delete
it.

#### Scenario: A lone group the summary already counts

- **WHEN** the page renders a form every question of which demands a written answer
- **THEN** the questions are listed without a heading, the summary above having
  already said how many written answers there are

#### Scenario: A lone group of one-line questions

- **WHEN** the page renders a form every question of which is answerable in a line
- **THEN** the questions are listed without a heading, no kind having been named for
  them anywhere before this change either

#### Scenario: A lone group whose kind nothing else names

- **WHEN** the page renders a form every question of which is answered by choosing
  from a list
- **THEN** the heading is shown, being the only place the reader is told these
  questions are answered by choosing rather than by writing

Within a group the questions SHALL keep the order the employer put them in. The
served order is not reordered — only partitioned — so that the employer's sequence
survives wherever grouping does not override it.

#### Scenario: A form spanning several kinds

- **WHEN** the page renders a form whose questions expect several kinds of answer
- **THEN** each kind appears as its own group, headed by that kind and its count,
  and the groups run from the cheapest kind to the most expensive

#### Scenario: A kind no question expects

- **WHEN** the page renders a form no question of which demands a file
- **THEN** no heading for attachments appears

#### Scenario: The employer's order within a group

- **WHEN** a group holds more than one question
- **THEN** they appear in the order the served form listed them

#### Scenario: A question whose kind was not named

- **WHEN** the page renders a question the projection named no answer kind for
- **THEN** it appears among the questions answerable in a line, which is what a
  reader assumes of an unqualified question

### Requirement: A question's row carries only what its group does not

Where a question is shown under a group heading, the page SHALL NOT repeat the
kind of answer on the question's own row; the heading has already said it.

The page SHALL continue to mark a question the platform will accept the
application without, and SHALL mark nothing on a question it requires — a required
question is the ordinary case, and marking it would put a word on nearly every
row to say that nothing unusual is true of it.

#### Scenario: An optional written answer

- **WHEN** an optional question demanding a written answer is shown under its
  group's heading
- **THEN** its row is marked optional and says nothing about expecting a written
  answer

#### Scenario: A required question

- **WHEN** a required question is shown
- **THEN** its row carries no mark

### Requirement: The provider is shown with its own mark where one is known

The page SHALL show the platform the form was read from, and SHALL show that
platform's brand mark alongside its name where a mark is known for it.

The mark SHALL accompany the name rather than replace it. Marks are known for some
platforms and not others, and a block identified by a mark alone would name its
source for some postings and leave it unnamed for the rest.

A platform no mark is known for SHALL be shown by name alone, with no placeholder
in the mark's place.

#### Scenario: A platform with a known mark

- **WHEN** the page renders a form read from a platform a mark is known for
- **THEN** the mark appears beside the platform's name

#### Scenario: A platform with no known mark

- **WHEN** the page renders a form read from a platform no mark is known for
- **THEN** the platform's name appears alone, and the line is otherwise unchanged

