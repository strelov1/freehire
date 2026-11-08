# search-suggestions Specification

## Purpose

TBD - created by archiving change add-search-suggestions. Update Purpose after archive.

## Requirements

### Requirement: Titles are mined from the catalogue above a frequency floor

The builder SHALL walk the open catalogue, normalise each posting title, count the
occurrences of each normalised form, and keep only those at or above a minimum
occurrence floor. A title carried by a single posting is noise, not a suggestion.

Normalisation SHALL lowercase the title, collapse whitespace, and cut it at the
first separator — a pipe, an opening bracket, a comma, a slash, an em dash, or a
literal " at ".

Measured over a 2,000-title sample of the live catalogue, 1,251 normalised titles
were distinct (62.5%) while the 204 occurring twice or more covered 47.6% of the
sample: the distribution is concentrated enough that a floor bounds the dictionary
rather than merely trimming it.

#### Scenario: A one-off title is not offered

- **WHEN** exactly one open posting carries a given normalised title
- **THEN** that title is absent from the index

#### Scenario: A title is cut at its first separator

- **WHEN** the posting title is `Senior Software Engineer, Infrastructure, Infra Spanner`
- **THEN** the mined title is `senior software engineer`

### Requirement: A frequent title that names no craft is still dropped

A frequency floor alone does not make a title useful. The same sample put bare
`manager` at 44 occurrences and bare `director` at 18 — frequent, and worthless as
a suggestion, because they name a grade rather than a craft.

The builder SHALL drop a normalised title that reduces to a bare seniority word
(the `vocab.SeniorityValues` surface forms) or to a bare generic
(`manager`, `director`, `consultant` standing alone), regardless of its count. The
role and category dictionaries carry those axes properly.

#### Scenario: A bare grade word is dropped despite being frequent

- **WHEN** the normalised title `manager` occurs far above the frequency floor
- **THEN** it is absent from the index

#### Scenario: The same word qualified by a craft is kept

- **WHEN** the normalised title is `engineering manager`
- **THEN** it is kept, because it names a craft

### Requirement: A suggestion with no open postings is not offered

The index is rebuilt from the open catalogue, so a suggestion it carries has
postings behind it by construction. The endpoint SHALL NOT offer a suggestion
whose posting count has fallen to zero between rebuilds: a suggestion that leads
to an empty result page is worse than no suggestion.

#### Scenario: A drained suggestion is withheld

- **WHEN** a suggestion in the index carries a posting count of zero
- **THEN** it is not offered

### Requirement: Companies are offered above a posting floor

The builder SHALL emit a `company` suggestion for each company slug carrying at
least a minimum number of open postings, so the long tail of one-off slugs does
not drown the dictionary. Each SHALL carry its open-posting count, which is what
separates one employer's spellings from each other — Google carries 3,187 while
Google India carries 19.

#### Scenario: A company below the floor is not offered

- **WHEN** a company slug carries fewer open postings than the floor
- **THEN** no company suggestion is emitted for it

#### Scenario: Distinct company slugs are offered separately with their counts

- **WHEN** `google` and `google-india` are both above the floor
- **THEN** both are offered, each naming its own posting count

### Requirement: The builder writes the index atomically

The builder SHALL write the suggestions index through the same rebuild-and-swap
the jobs index rebuild uses, so a reader never observes a partially built
dictionary. It SHALL be a run-once-and-exit worker that exits non-zero on failure.

#### Scenario: A failed build leaves the previous index serving

- **WHEN** the builder fails part way through
- **THEN** the previously built index continues to serve and the worker exits non-zero

### Requirement: The suggest endpoint completes the trailing fragment

The system SHALL expose `GET /api/v1/suggest?q=`, returning at most ten
suggestions in the list response shape.

The endpoint SHALL split the query into a **recognised prefix** and a **trailing
fragment**. The prefix is the longest run of leading tokens that matches a
normalised phrase in the dictionary; the fragment is everything the prefix did not
consume. Completions SHALL be computed for the fragment, and each returned row
SHALL name the whole phrase — the recognised prefix plus the candidate — together
with every part it would apply.

Recognition of the prefix SHALL be exact, without typo tolerance: a mistyped
phrase MUST fall through into the fragment rather than be silently consumed as
recognised, because the fragment is where typos are forgiven.

#### Scenario: A trailing fragment completes against a company

- **WHEN** the query is `senior software engineer go`
- **THEN** a row offers "Senior Software Engineer Google", naming both the role part and the company part

#### Scenario: A mistyped phrase is not consumed as recognised

- **WHEN** the query is `senior sofware enginer`
- **THEN** the misspelled words are treated as the fragment, and the completion still reaches Senior Software Engineer

### Requirement: A kind already named by the prefix is not offered again

Completions for the fragment SHALL exclude the kinds the recognised prefix has
already filled: a query that has named a role SHALL NOT be offered a second role,
and one that has named a company SHALL NOT be offered a second company. Skills are
the exception — several skills narrow a search sensibly, so that kind stays open.

#### Scenario: A second role is not offered

- **WHEN** the recognised prefix names a role and the fragment would match another role
- **THEN** no role is among the completions

#### Scenario: A second skill is offered

- **WHEN** the recognised prefix names the skill Java and the fragment matches the skill Kubernetes
- **THEN** Kubernetes is among the completions

### Requirement: Choosing a suggestion applies every part it names

Choosing a suggestion SHALL apply all of its parts in one interaction. A composed
row naming a role and a company SHALL set the role facet and `company_slug`
together; applying one of the two would silently discard what the visitor typed.

A `title` part SHALL be applied as the query, since no facet names it — see
"Applying a title suggestion narrows to a quoted, title-scoped query" below for
how that query is built. Facets combine with AND, so applying several parts
narrows rather than replaces.

#### Scenario: A composed row applies both parts

- **WHEN** the visitor chooses "Senior Software Engineer Google"
- **THEN** the role facet and `company_slug=google` are both applied

#### Scenario: A title row applies as a query

- **WHEN** the visitor chooses "Java Developer" of kind `title`
- **THEN** the query is built from `java developer` (quoted and restricted to
  the title field) and no facet is set

### Requirement: An empty query returns a curated entry point

An empty `q` SHALL return category suggestions drawn from the curated group order
the filter modal already uses — Engineering, Data & AI, Quality & Security, Design
& Creative, Product & Management, Go-to-market & Support, People, Business &
Legal, then the consumer industries.

It SHALL NOT return the highest-count values. Measured on the live catalogue those
are Management (266,883), Sales (179,993) and Support (127,110), which read as a
different website to a visitor who came for engineering work.

Each group SHALL contribute its **two** busiest measured categories before the
next group is reached, up to ten rows. Neither a flat walk of the order nor one
row per group works:

- A flat walk spends every row on Engineering, which carries 13 categories on its
  own, and never reaches a designer or a PM.
- One per group flattens the order into a map of a catalogue that is only half a
  tech catalogue. Measured against production it spent five of the ten rows on
  Management, Sales, HR, Operations and Healthcare.

Two per group spends the budget on the groups the curated order puts first. The
rest stay one keystroke and one filter pane away — this list is a starting point,
not the vocabulary.

A category the distribution does not carry SHALL NOT be offered, and the catch-all
`other` SHALL NOT be offered at all: it names no craft, so it cannot answer the
question an empty box is asking.

#### Scenario: The empty box leads with engineering

- **WHEN** the query is empty
- **THEN** the first suggestions come from the Engineering group, not from the highest-count categories

#### Scenario: A leading group gets two rows before a later one is reached

- **WHEN** the query is empty and every group has measured categories
- **THEN** two Engineering categories and two Data & AI categories are offered before any Go-to-market category

#### Scenario: A group that cannot fill two rows does not hold the others back

- **WHEN** a leading group has only one measured category
- **THEN** it contributes that one and the next group follows immediately

#### Scenario: The catch-all is never a starting point

- **WHEN** the query is empty and the `other` category has more postings than any named one
- **THEN** `other` is not among the suggestions

#### Scenario: The empty box is never empty

- **WHEN** the query is empty
- **THEN** at least one suggestion is returned

### Requirement: A typo is forgiven in the fragment

Completions for the fragment SHALL be typo-tolerant, and a match reached only
through typo tolerance SHALL be offered rather than dropped. Ranking SHALL place
the intended target first.

The previous rule dropped such matches because it ranked them by open-vacancy
count, which put Marketing Specialist (55,768, reached by edit distance against
its `growth hacker` alias) above Backend Engineer for `backedn`. The index ranks
by relevance to the fragment, so the reason no longer holds.

#### Scenario: A transposition reaches its target

- **WHEN** the query is `backedn`
- **THEN** Backend Engineer is the first suggestion

#### Scenario: A typo does not surface an unrelated large bucket

- **WHEN** the query is `backedn`
- **THEN** Marketing Specialist is not offered above Backend Engineer

### Requirement: Suggestions are ranked by demand, then by supply

The endpoint SHALL rank completions by recorded search frequency first, then by
open-posting count, then by shorter text. Demand orders what people actually ask
for; supply breaks the tie among suggestions nobody has asked for yet.

#### Scenario: A more-searched suggestion leads

- **WHEN** two suggestions match equally well and one has been searched for more often
- **THEN** the more-searched one is offered first

#### Scenario: Supply breaks a tie with no demand recorded

- **WHEN** two suggestions match equally well and neither has been searched for
- **THEN** the one with more open postings is offered first

### Requirement: The suggest endpoint carries its own rate limit

`/api/v1/suggest` is called once per keystroke, which is an order of magnitude
more requests than the search box issues today. It SHALL be rate limited in its
own bucket, keyed per client, so its volume cannot exhaust the allowance the other
public reads share.

#### Scenario: Suggest volume does not throttle job search

- **WHEN** a client exhausts its suggest allowance
- **THEN** its requests to the job search endpoint are still served

### Requirement: A dedicated suggestions index holds every offerable completion kind

The system SHALL maintain a Meilisearch index, separate from the jobs index, holding
one document per offerable suggestion. Each document SHALL carry the display text, a
`kind` of `title`, `skill`, `category` or `company`, the facet value the suggestion
applies (absent for a `title`, which is free text), the count of open postings behind
it, and the count of times visitors have searched for it.

The index SHALL be separate rather than a facet on the jobs index. A facet is a bounded
value dictionary and distinct job titles number in the millions: `MaxValuesPerFacet`
would truncate the distribution and `title` is not a filterable attribute. Suggestions
are mined into a bounded dictionary offline instead.

#### Scenario: A title suggestion carries no facet value

- **WHEN** the index holds the suggestion "Java Developer" of kind `title`
- **THEN** it carries no facet value, because no facet names that phrase

#### Scenario: A category suggestion carries its slug

- **WHEN** the index holds the suggestion "Backend" of kind `category`
- **THEN** it carries the facet value `backend`

#### Scenario: No suggestion names a role

- **WHEN** the dictionary is built
- **THEN** no document carries the kind `role`

### Requirement: Applying a title suggestion narrows to a quoted, title-scoped query

A title suggestion's displayed job count is mined as an exact match on the
posting's normalised title. Applying the suggestion SHALL therefore search with
its text quoted (every word required, in any order, with typo tolerance off)
and restricted to the job title field, rather than as an unscoped multi-field
query — so the count a visitor saw approximates the results the click
produces, instead of a materially larger set drawn from matches anywhere
across title, company, description, and location. This is a closer
approximation, not exact parity: quoting in this deployment requires every
word to be present rather than verifying they are adjacent (word-adjacency
verification needs a different Meilisearch proximity configuration and a full
reindex, out of scope here — see the `search-q-field-scoping` capability).

This requirement covers title suggestions only. A skill, category, or company
suggestion already applies as an exact facet filter against the same index its
count was drawn from, so its displayed count already matches what a click
produces and neither its application nor its count computation changes.

#### Scenario: A title suggestion's count approximates its click-through results

- **WHEN** a visitor applies a title suggestion
- **THEN** the resulting job search is a quoted query restricted to the title
  field, and the number of results it returns is close to the job count the
  suggestion displayed, not a materially larger set drawn from unrelated
  matches in company, description, or location

#### Scenario: A composed suggestion's title part is quoted and title-scoped

- **WHEN** a visitor applies a suggestion carrying both a title part and a
  facet part (for example, a role at a named company)
- **THEN** the title part is applied as a quoted query restricted to the title
  field, alongside the facet filter for the other part

#### Scenario: A suggestion-originated search still counts as ordinary demand

- **WHEN** a title suggestion's quoted, field-restricted search is recorded
  for the suggestion dictionary's demand tracking
- **THEN** it is recorded under the same normalised phrase a visitor typing the
  same words unquoted would produce, not as a distinct phrase carrying the
  search's own quoting or field-restriction notation

#### Scenario: A non-title suggestion's count is unaffected

- **WHEN** a visitor applies a skill, category, or company suggestion
- **THEN** it is applied as an exact facet filter, exactly as before this
  change, and its displayed count is unaffected

#### Scenario: The restriction applies from the launcher and from a list page

- **WHEN** a visitor applies a title suggestion while already viewing a job list
  (`/jobs`, a company's job list, a role/country page, or a collection), not
  only when navigating to one from elsewhere
- **THEN** the search that list runs is quoted and restricted to the title
  field, the same as a launcher-driven navigation to `/jobs`

#### Scenario: The restriction survives further interaction with that same list

- **WHEN** a visitor pages forward, changes a facet, or changes the sort order
  on a list whose current search came from applying a title suggestion
- **THEN** the request for the next page/facet-count/sort still carries the
  same quoted, title-restricted query — it does not silently widen back to an
  unscoped, multi-field match

#### Scenario: Typing a new query clears an inherited restriction

- **WHEN** a visitor edits the search box directly (typing, the header's
  Enter, or its clear button) after a title suggestion's restriction is
  active
- **THEN** the new search is NOT restricted to the title field — the
  restriction was scoped to the suggestion that set it, not a general
  preference for future searches

#### Scenario: The quoting used to build the request never reaches a person

- **WHEN** a title suggestion's quoted query is shown to a visitor — a
  filter chip, the header search box, an analytics event describing what
  was searched
- **THEN** it reads as plain text with no quote marks, even though the
  request actually sent to search carries them
