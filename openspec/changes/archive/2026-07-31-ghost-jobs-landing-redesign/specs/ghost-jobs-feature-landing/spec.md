## ADDED Requirements

### Requirement: Public ghost-jobs feature landing page

The frontend SHALL serve a public, unauthenticated page at `/features/ghost-jobs` that
explains the ghost signal to a reader who is not signed in, and SHALL render its copy
server-side so the initial HTML carries the explanatory text without client JavaScript.

The page SHALL be composed of, in order: a hero, a criteria section, a section explaining
how the level is decided, a section on how a reader contributes evidence, a section stating
the limits of the system's coverage, a FAQ, and a closing call to action.

The hero SHALL carry the contrast between what the system can check and what it never
claims. That contrast SHALL appear above any explanation of the mechanics: the limit on the
claim is the feature's most important property rather than a disclaimer, and a reader who
stops after the hero must have read it.

#### Scenario: Page is public and server-rendered

- **WHEN** an anonymous visitor requests `GET /features/ghost-jobs`
- **THEN** the response is 200 and its HTML body already contains the page headline and
  section copy, with no sign-in redirect

#### Scenario: The limit is stated before the mechanics

- **WHEN** the page renders
- **THEN** the statement of what the system never claims appears in the hero, above the
  section explaining which criteria fire

### Requirement: Every criterion the classifier weighs carries its own diagram

The page SHALL render one diagram per criterion in the classifier's vocabulary, drawn from
the same array the product's checklist reads, so a criterion cannot join the vocabulary and
appear on the page as an empty cell.

Each criterion SHALL be presented as its diagram, the criterion's name, an example of the
observations behind it, and a short summary, with the full explanation available through a
disclosure rather than deleted.

#### Scenario: A criterion without a diagram fails the build

- **WHEN** a criterion is present in the classifier vocabulary with no diagram registered
- **THEN** the type check fails, rather than the page rendering a criterion with no
  illustration

#### Scenario: The criteria are grouped by tier

- **WHEN** the criteria section renders
- **THEN** the structural criteria and the outcome criteria are presented as two labelled
  groups, so the reader can see which kind of evidence they are looking at

### Requirement: The evergreen diagram depicts convergence, not age

The diagram for the evergreen criterion SHALL depict reposting and concurrent duplicate
postings as what makes the criterion fire, and MUST NOT depict elapsed time as the trigger.

A criterion whose example text opens with a posting's age invites a time axis, but age
alone does not fire it, and in the live catalogue effectively never does — the marked
postings converge through repost and duplicate-copy thresholds instead. A diagram
foregrounding a timeline would tell the reader the wrong reason their posting is marked,
which is the one question that brought them to the page.

#### Scenario: The diagram foregrounds duplicate copies

- **WHEN** the evergreen criterion's diagram renders
- **THEN** it depicts multiple concurrent copies of one posting and a repost count, and
  presents no time axis as the criterion's cause

### Requirement: The prevalence figure is shown as a range

The page SHALL present the share of listings that are not being worked as the published
range rather than a single number, and the graphic depicting it SHALL distinguish the
lower bound from the uncertain band above it.

A single averaged figure would claim a precision the underlying studies do not have, on a
page whose entire argument is that it states only what it can check.

#### Scenario: The band is visible as a band

- **WHEN** the prevalence graphic renders
- **THEN** the cells representing the lower bound are distinguishable from those
  representing the remainder of the range

### Requirement: Diagrams inherit the signal's no-reassurance colour rule

A diagram element standing for a criterion that did not fire, for evidence that was not
counted, or for a check that was not performed SHALL be rendered in a neutral tone, and
MUST NOT be rendered in a tone that reads as reassurance.

This is the constraint that already governs the gauge's uncoloured segments, and it binds
the page's diagrams for the same reason: the system distinguishes a criterion that fired
from one that did not, and never a criterion checked and found clear from one with nothing
to check. A reassuring colour would assert a difference the data does not carry.

#### Scenario: An uncounted lane claims nothing

- **WHEN** a diagram depicts evidence that was not counted, such as an application whose
  owner has no connected mailbox
- **THEN** that element renders in a neutral tone rather than one reading as cleared or safe

#### Scenario: An unjudged criterion is shown as unjudged

- **WHEN** the diagram for the company-board criterion renders
- **THEN** it distinguishes three states — the role found, the role absent from a board the
  system crawls, and a company whose board the system does not crawl and therefore does not
  judge at all

### Requirement: Counts below the anonymity gate are drawn as absent

A diagram depicting outcome evidence below the contributor gate SHALL render the count as
absent rather than as a zero or a suppressed value.

The served payload omits the field entirely below the gate, because a count of one
identifies the single person who applied to the employer. A diagram showing "0" or a
redacted placeholder would depict a system that holds the number and withholds it, which is
not the system that exists.

#### Scenario: One contributor renders no count

- **WHEN** a diagram or the sandbox depicts outcome evidence from a single contributor
- **THEN** no count is rendered, and the space where a count would sit is shown as carrying
  no value

### Requirement: Diagrams are decorative to assistive technology

Each diagram SHALL be hidden from assistive technology, and the criterion's name, example
observations and summary SHALL sit beside it as text carrying the same information.

A screen reader gains nothing from an unlabelled arrangement of shapes, and the page already
states every fact the diagrams depict in prose. This follows the treatment the product's own
gauge receives for the same reason.

#### Scenario: The diagram is skipped and nothing is lost

- **WHEN** a screen reader traverses the criteria section
- **THEN** the diagrams are not announced, and the criterion's name, example and summary are

### Requirement: Long-form copy is disclosed rather than deleted

The page SHALL keep the full explanation of each criterion and the full FAQ in the served
HTML, presenting them behind disclosure controls that are collapsed on first paint.

Disclosures SHALL work without client JavaScript, so a server-rendered first paint is
operable and the collapsed text is present for the page's structured data.

The FAQ SHALL continue to render from the same array that builds the page's FAQ structured
data, so the two cannot disagree.

#### Scenario: Collapsed text is still served

- **WHEN** an anonymous visitor requests the page and does not expand anything
- **THEN** the HTML body contains the full text of every FAQ answer and every criterion
  explanation

#### Scenario: A disclosure opens without client JavaScript

- **WHEN** a reader activates a disclosure control on a page whose client JavaScript has not
  run
- **THEN** the content expands

#### Scenario: The FAQ and its structured data share one source

- **WHEN** a FAQ entry is added, edited or removed
- **THEN** both the visible FAQ and the page's FAQ structured data change together, because
  they render from the same array
