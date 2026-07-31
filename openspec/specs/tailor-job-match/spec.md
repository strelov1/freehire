# tailor-job-match Specification

## Purpose
Tell a candidate how well the CV they are editing right now matches the vacancy it was written
for — measured on the artifact a parser reads (the rendered PDF's text layer), against that
vacancy and nothing else. Deterministic and dictionary-only, so it calls no model, costs no AI
credits, and can be recomputed every time the document changes. Where a dictionary resolves
nothing the answer is "unverifiable": such an input leaves the denominator and never the
numerator, because a check we could not run is not a failure the candidate caused. It is
informational and gates nothing.

## Requirements
### Requirement: The job-match score measures the tailored document against its vacancy alone

The system SHALL score a tailored CV against the vacancy it is bound to, using only that vacancy
and the tailored document as inputs. It MUST NOT read the base CV, the experience bank, or the
candidate's structured résumé: those describe the candidate, and this score describes the
document.

The scoring input SHALL be the text layer extracted from the tailored CV rendered to PDF with its
own template and page margins — the same artifact the ATS-readiness score reads, for the same
reason: a field the active template never renders contributes nothing to what a parser sees.

The score SHALL be deterministic and SHALL NOT call a language model. It therefore consumes no AI
credits and MAY be recomputed as often as the document changes.

#### Scenario: The score reads the rendered text layer

- **WHEN** the job-match score is computed for a tailored CV
- **THEN** it is computed from text extracted from the rendered PDF, and a document field the active template does not render contributes nothing

#### Scenario: The base CV does not enter the score

- **WHEN** the job-match score is computed
- **THEN** the base CV is neither read nor rendered, and the score is unchanged by any edit to it

#### Scenario: Scoring makes no model call

- **WHEN** the job-match score is computed with no LLM configured in the environment
- **THEN** the score is produced in full and no AI credit is consumed

### Requirement: Four weighted categories, and the response carries their weights

The score SHALL be composed of exactly four categories, each carrying a fixed weight out of 100:

| Category | ID | Weight |
|---|---|---|
| Requirements Coverage | `requirements_coverage` | 40 |
| Keyword Match | `keyword_match` | 30 |
| Job Title Match | `job_title_match` | 20 |
| Seniority Fit | `seniority_fit` | 10 |

Every category SHALL report its earned points, its weight, and the line items that produced them.
The weight SHALL be part of the wire shape rather than a constant the client re-declares, so a
re-weighting is a server change and the UI's impact labelling can never disagree with the score.

#### Scenario: Each category reports its weight alongside its score

- **WHEN** a job-match score is served
- **THEN** each of the four categories reports its id, its label, its earned points, its weight, and its line items

#### Scenario: The weights are the server's

- **WHEN** the client renders a category's impact
- **THEN** it renders the weight the response carried, and no weight is hard-coded in the client

### Requirement: An unverifiable input leaves the denominator, never the numerator

The overall score SHALL be the earned points across all **available** categories divided by the sum
of those categories' weights, expressed out of 100. A category the system cannot evaluate SHALL be
reported as unavailable with a reason, and SHALL be excluded from both the numerator and the
denominator. It MUST NOT be scored as zero: an input we cannot check is not a failure the candidate
caused.

The same rule SHALL apply within a category: an individual check that cannot be evaluated is
excluded from that category's own denominator, and a category all of whose checks are unevaluable
SHALL itself be reported unavailable.

The response SHALL name which categories contributed, so a score computed over three categories is
never mistaken for one computed over four.

#### Scenario: An unavailable category is excluded from both sides

- **WHEN** Requirements Coverage cannot be evaluated and the other three categories earn 45 of their 60 combined weight
- **THEN** the overall score is 75, and the response names Requirements Coverage as unavailable with a reason

#### Scenario: An unavailable category is not a zero

- **WHEN** a category is unavailable
- **THEN** the overall score is strictly greater than it would be if that category had scored zero at full weight

#### Scenario: All four available scores over the full weight

- **WHEN** every category is available
- **THEN** the denominator is 100 and the overall score is the plain sum of the four earned scores

### Requirement: Requirements Coverage is re-derived from the current document, never re-asserted

Requirement *texts* SHALL be read from the cached fit analysis for this (user, vacancy): they
describe the posting and do not depend on any CV. Their covered/missing status SHALL be recomputed
against the current tailored document and MUST NOT be copied from the cached analysis, whose
statuses were determined against the candidate's base profile.

A requirement's skills SHALL be **the vacancy's own canonical skills that the requirement's text
names** — not whatever the dictionary can find in that text read alone. The vacancy's skills were
resolved from its full description, where an ambiguous term had the surrounding context needed to
resolve it; a single requirement line has no such context, and tagging it in isolation both drops
skills it plainly states and risks inventing ones it does not.

A requirement SHALL therefore be re-derived as follows:

- a requirement naming at least one of the vacancy's canonical skills is **checkable** — it counts
  as covered when every skill it names is present in the document's parsed skills, and missing
  otherwise;
- a requirement naming none of them is **unverifiable**. It SHALL be reported as such, carrying the
  cached analysis's status labelled as coming from the earlier analysis, and SHALL be excluded from
  the category's denominator per the unverifiable rule.

A requirement MUST NOT be attributed a skill the vacancy does not itself carry. Requirements
Coverage and Keyword Match therefore draw from one set, and cannot disagree about what the vacancy
asks for.

A `required` requirement SHALL count for more than a `preferred` one within the category.

When no fit analysis is cached for the pair, the category SHALL be reported unavailable with a
reason naming that, rather than reported as zero coverage.

#### Scenario: A skill-bearing requirement follows the current document

- **WHEN** a cached requirement names a canonical skill the base profile lacked but the tailored document now contains
- **THEN** it is reported covered, regardless of the status the cached analysis recorded

#### Scenario: A requirement naming no vacancy skill is not reported missing

- **WHEN** a cached requirement's text names none of the vacancy's canonical skills (for example, "strong communication skills")
- **THEN** it is reported unverifiable, is excluded from the category's denominator, and its cached status is shown labelled as coming from the earlier analysis

#### Scenario: A skill the vacancy states is read even where it needs context to resolve

- **WHEN** a vacancy carrying the canonical skill `go` has the requirement "5+ years of Go"
- **THEN** that requirement is checkable against `go`, even though the line read on its own carries no other technical term to resolve the ambiguity against

#### Scenario: A requirement is never attributed a skill outside the vacancy

- **WHEN** a requirement's text mentions a skill the vacancy does not carry
- **THEN** that skill does not enter the requirement's check, and Requirements Coverage draws only from the set Keyword Match draws from

#### Scenario: A required requirement outweighs a preferred one

- **WHEN** two documents cover the same number of requirements, one covering only `required` ones and the other only `preferred` ones
- **THEN** the first scores higher in Requirements Coverage

#### Scenario: No cached analysis makes the category unavailable

- **WHEN** the score is computed for a pair with no cached fit analysis
- **THEN** Requirements Coverage is reported unavailable with a reason, and the overall score is computed over the remaining three categories

### Requirement: Keyword Match names the vacancy's missing skills

Keyword Match SHALL compare the vacancy's canonical skills against the skills parsed from the
tailored document's rendered text, using the same dictionary and the same résumé-acronym handling
the ATS-readiness score uses — so the two surfaces can never disagree about which skills the
document contains.

The category SHALL report the matched skills and the missing ones by name, so the candidate is told
what to add rather than only how far short they fell. A vacancy carrying no canonical skills SHALL
make the category unavailable rather than award full or zero marks.

#### Scenario: Missing skills are named

- **WHEN** the vacancy lists canonical skills the document does not contain
- **THEN** the category reports each missing skill by name

#### Scenario: The two surfaces agree on the document's skills

- **WHEN** both the ATS-readiness score and the job-match score are computed for the same document
- **THEN** both derive the document's skills from the same parse of the same rendered text

#### Scenario: A vacancy with no canonical skills makes the category unavailable

- **WHEN** the vacancy has no canonical skills
- **THEN** Keyword Match is unavailable with a reason and is excluded from the denominator

### Requirement: Job Title Match and Seniority Fit are dictionary-only

Job Title Match SHALL be derived from the vacancy's title and the tailored document's rendered
text using the existing title classification dictionaries. It SHALL award full marks when the
vacancy's title appears in the document, partial marks when the document carries a title of the
same role category, and no marks otherwise.

Seniority Fit SHALL compare the seniority the dictionaries derive from the vacancy's title against
the seniority they derive from the document. A match SHALL award full marks and an adjacent level
partial marks. When either side yields no seniority, the check SHALL be unverifiable rather than a
guess — the dictionaries emit nothing for unknowns here as everywhere else.

#### Scenario: An exact title earns full marks

- **WHEN** the document's text contains the vacancy's title
- **THEN** Job Title Match awards its full weight

#### Scenario: A same-category title earns partial marks

- **WHEN** the document carries no matching title but carries one the dictionaries place in the vacancy's role category
- **THEN** Job Title Match awards partial marks and its line item says which title it matched

#### Scenario: An unreadable seniority is unverifiable, not a miss

- **WHEN** the vacancy's title yields no seniority from the dictionaries
- **THEN** Seniority Fit is unavailable with a reason rather than scored as a mismatch

### Requirement: The score is computed on read, never stored, and is owner-scoped

The score SHALL be computed per request from the current document, the current vacancy and the
current scoring rules, and no part of it SHALL be persisted. A scoring-rule change SHALL therefore
be reflected on the next read with no migration or invalidation step.

The read SHALL be owner-scoped: a CV belonging to another account SHALL be indistinguishable from
one that does not exist. A CV that is not a tailored copy, and a tailored copy whose vacancy no
longer exists, SHALL each be refused as a conflict naming which case it is — not answered with a
fabricated score.

#### Scenario: Another account's CV is not found

- **WHEN** a caller reads the score for a CV owned by a different account
- **THEN** the response is the same not-found as for a CV id that does not exist

#### Scenario: A base CV has no job-match score

- **WHEN** a caller reads the score for the base CV
- **THEN** the response is a conflict saying it is not bound to a vacancy, and no score is computed

#### Scenario: An edit is reflected on the next read

- **WHEN** the tailored CV is edited and the score is read again
- **THEN** the new response reflects the edit, and no stored score had to be invalidated

### Requirement: An unavailable renderer degrades the score, not the workspace

When the score cannot be computed because the Typst renderer or the PDF text extractor is
unavailable, or a render fails, the response SHALL report the score as unavailable with a reason
and a success status. It MUST NOT return a server error and MUST NOT prevent the tailoring
workspace from loading or the CV from being edited.

#### Scenario: A missing renderer yields an unavailable score

- **WHEN** the CV renderer is not configured in the running environment
- **THEN** the response reports the score unavailable with a reason and a success status

#### Scenario: A failed render does not fail the workspace

- **WHEN** rendering the tailored CV fails
- **THEN** the score is reported unavailable and the workspace continues to load and edit the CV

### Requirement: The workspace surfaces the score live and keeps it current

The tailoring workspace SHALL present the job-match score in its own Job Match tab, and SHALL
refresh it at every moment the document has changed: when the workspace opens, after an agent turn
completes, and after an edit is persisted by autosave.

The tab SHALL show the overall score, the vacancy it was scored against, and each category as a
disclosure that expands to the line items behind its score. A category's impact SHALL be rendered
from the weight the response carried.

An unavailable score SHALL render as an absence, not as an error state.

#### Scenario: An autosaved edit refreshes the score

- **WHEN** a human edit is persisted by autosave
- **THEN** the job-match score is requested again and the displayed values reflect the edit

#### Scenario: A completed agent turn refreshes the score

- **WHEN** an agent turn finishes
- **THEN** the job-match score is requested again and the displayed values reflect the turn's edits

#### Scenario: A category expands to its line items

- **WHEN** the user expands a category row
- **THEN** the line items that produced its score are shown, each with its own status and points

#### Scenario: An unavailable score shows nothing rather than an error

- **WHEN** the score is reported unavailable
- **THEN** the tab displays no score and no error state

### Requirement: The frozen fit analysis is labelled as a snapshot of the base profile

The Job Match tab SHALL continue to present the cached LLM fit analysis beneath the live score,
and SHALL label it as measuring the candidate's base profile rather than the document being edited,
alongside its existing recompute control.

An unlabelled fit score beside a live one teaches the candidate that tailoring does not move the
number, which is true of that score and false of the surface as a whole.

#### Scenario: The snapshot is labelled

- **WHEN** the Job Match tab renders the cached fit analysis
- **THEN** it is labelled as measuring the base profile, not the tailored document

#### Scenario: The two scores are visually distinct

- **WHEN** both the live job-match score and the cached fit score are shown
- **THEN** they are presented as separate blocks with their own headings, not as two rows of one table
