## ADDED Requirements

### Requirement: A single-skill filter collection links to its glossary entry

A filter collection whose `params` pin exactly one facet, `skills`, to a single scalar value
SHALL link to that skill's glossary page. The link SHALL be absent for every other collection.

This is the reciprocal of the link the glossary now carries to the collection, and the third
side of a division of labour the landing pages already keep between them: the collection page
answers "who is hiring for this", the role landing it already links to answers "where are they
and what do they pay", and the glossary answers "what is it". Each of the three is a distinct
search intent and a distinct page, so linking them is not duplication — leaving them unlinked
is what makes two of them look like competing answers to one question.

The condition SHALL be read from the collection's own `params`, never from its slug matching a
skill's. Four slugs name both a skill and a collection while the collection pins a **category**
(`data-science`, `data-engineering`, `devops`, `machine-learning`); a category feed and a skill
facet are different sets, so a slug match would publish a link promising one and delivering the
other. The scalar condition matters for the same class of reason: a list-valued `skills` pin
expands to repeated keys with OR semantics, which is not any single skill's set.

Resolving the link SHALL require no description lookup. Every canonical skill carries a
glossary entry, so a pinned `skills` value — which is a canonical facet value — has a page by
construction, and the collection page pays no fetch to discover it.

#### Scenario: A collection pinning one skill

- **WHEN** a reader opens `/collections/kotlin`, whose params are `{ skills: 'kotlin' }`
- **THEN** the page links to that skill's glossary entry

#### Scenario: A collection pinning a category that shares a skill's name

- **WHEN** a reader opens `/collections/devops`, whose params are `{ category: 'devops' }`
- **THEN** the page carries no glossary link, because the feed is not the skill's set

#### Scenario: A collection pinning more than one facet

- **WHEN** a reader opens a collection pinning a skill alongside a region
- **THEN** the page carries no glossary link, because the feed is narrower than the skill

#### Scenario: A company-membership collection

- **WHEN** a reader opens a collection whose membership is company-level rather than a filter
- **THEN** the page carries no glossary link
