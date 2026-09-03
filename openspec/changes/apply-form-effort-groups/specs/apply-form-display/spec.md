## ADDED Requirements

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

A group no question falls into SHALL NOT be shown. Where every question falls into
a single group the headings SHALL be omitted entirely, because a lone heading
would only repeat the count already stated above it.

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

#### Scenario: A form of one kind only

- **WHEN** the page renders a form every question of which expects the same kind of
  answer
- **THEN** the questions are listed without any group heading

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
