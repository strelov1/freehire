## ADDED Requirements

### Requirement: Interpreting a search description into canonical facets

The system SHALL turn a natural-language description of a job search into facet values
that the existing search filter accepts verbatim, together with a one-sentence summary of
the search it built.

The interpretation SHALL be produced by a single model call, so the summary and the facet
values can never describe different searches.

#### Scenario: A description resolves to facets

- **WHEN** a caller submits "senior Go backend developer, remote, in Portugal"
- **THEN** the result carries `seniority: [senior]`, `category: [backend]`,
  `work_mode: [remote]`, `skills: [go]` and `countries: [pt]`
- **AND** the result carries a summary sentence describing that search

#### Scenario: The summary comes from the same call as the facets

- **WHEN** the model returns an interpretation
- **THEN** the summary is read from that same response
- **AND** no second model call is made to describe the result

### Requirement: No facet value reaches the filter unresolved

The search filter ignores values it does not recognise, so an unresolved value produces an
unfiltered result set that reads as a confident answer. The system SHALL therefore
canonicalise every value the model emits before it becomes a facet, and SHALL drop any
value no dictionary resolves.

Closed-vocabulary facets (`work_mode`, `seniority`, `category`, `employment_type`,
`role_type`, `english_level`, `education_level`, `relocation`, `regions`, `company_type`,
`company_size`, `salary_currency`, `salary_period`) SHALL be validated against
`internal/vocab`. Open-vocabulary facets SHALL be resolved through the dictionary that
already owns that vocabulary: `skilltag.Canonicalize` for skills, `location` for countries
and cities, `industrytag.Canonicalize` for domains, and the company-slug rule with its
alias registry for companies.

A facet name outside the published filter vocabulary SHALL be refused outright.

#### Scenario: An alias resolves to its canonical value

- **WHEN** the model emits the skill "Golang"
- **THEN** the result carries `skills: [go]`

#### Scenario: A country written as a name resolves to its code

- **WHEN** the model emits the country "Portugal"
- **THEN** the result carries `countries: [pt]`

#### Scenario: A value outside a closed vocabulary is dropped

- **WHEN** the model emits `seniority: [rockstar]`
- **THEN** the result carries no `seniority` facet

#### Scenario: An unknown facet name is refused

- **WHEN** the model emits a facet named `vibe`
- **THEN** the interpretation fails rather than returning a result that silently omits it

### Requirement: Dropped values are reported, never silently discarded

The system SHALL report every value it dropped, verbatim as the model wrote it, so the
caller can see what was not understood. A drop that is not reported is indistinguishable
to the caller from a value that was applied.

#### Scenario: An unresolvable skill is reported

- **WHEN** the model emits the skill "blockchain-adjacent", which no dictionary resolves
- **THEN** the result carries no such skill
- **AND** the result's unresolved list contains "blockchain-adjacent"

#### Scenario: Nothing resolved at all

- **WHEN** every value the model emitted was dropped
- **THEN** the result carries no facets, no query and no scalars
- **AND** the caller can tell this apart from a search that resolved successfully

### Requirement: Free text is a last resort, not a shortcut

The result MAY carry a free-text query, but only for a concept no facet expresses. A
free-text term that duplicates a facet narrows the results twice, so the system SHALL
instruct the model accordingly and SHALL expose the query separately from the facets so a
caller can remove it on its own.

#### Scenario: A concept with no facet becomes free text

- **WHEN** a caller asks for roles "working on climate modelling"
- **THEN** the result carries that concept as its free-text query

#### Scenario: A concept a facet covers does not become free text

- **WHEN** a caller asks for "remote" roles
- **THEN** the result carries `work_mode: [remote]` and no free-text query for it

### Requirement: Scalar bounds

The result MAY carry the scalar filters the search accepts — minimum salary, posted-within
days, maximum years of experience, and the visa-sponsorship flag. Each SHALL be bounded to
the range the search filter accepts; a value outside it SHALL be dropped and reported like
any other unresolved value.

#### Scenario: A freshness phrase becomes a bound

- **WHEN** a caller asks for roles "posted this week"
- **THEN** the result carries a posted-within bound of 7 days

#### Scenario: An out-of-range scalar is dropped

- **WHEN** the model emits a posted-within bound of 100000 days
- **THEN** the result carries no posted-within bound
- **AND** the drop appears in the unresolved list

### Requirement: The interpretation reads no saved profile

The system SHALL NOT build a search from the caller's saved profile. That capability
already exists on the client ("Apply my profile"), where it is a pure mapping: a profile
is validated into the filter's own vocabulary when it is saved, so turning one into a
search needs no model at all. Interpreting it here would be a second set of rules free to
diverge from the first, at the cost of a model call.

#### Scenario: The endpoint reads no profile

- **WHEN** an interpretation is requested
- **THEN** the caller's saved profile is not read, and the result depends only on the
  description they wrote

### Requirement: Refining an interpretation

A caller MAY refine a result by adding a constraint in words. The system SHALL run the
interpretation again with the previous result as context and return a complete replacement
result, so the caller always sees one coherent search rather than a diff.

Refinement SHALL be stateless: nothing about an interpretation or its refinements is
persisted.

#### Scenario: A constraint is added

- **WHEN** a caller refines a result of "senior Go backend, Portugal" with "remote only"
- **THEN** the new result carries the original facets plus `work_mode: [remote]`

#### Scenario: A refinement contradicts the previous result

- **WHEN** a caller refines a result carrying `work_mode: [onsite]` with "actually remote"
- **THEN** the new result carries `work_mode: [remote]` and not `onsite`

### Requirement: The endpoint

The system SHALL expose the interpretation as `POST /api/v1/search/interpret`.

It SHALL require a signed-in caller, be rate-limited per caller, and cap the submitted
text length. Every model call it makes SHALL go out on that caller's own gateway
credential under a dedicated feature tag, as every per-user model call does.

When search or the model client is unconfigured, the endpoint SHALL report the same
service-unavailable status the other search-dependent routes report.

#### Scenario: Unauthenticated

- **WHEN** a caller without a session posts to the endpoint
- **THEN** the response is 401

#### Scenario: Over the rate limit

- **WHEN** a caller exceeds the per-caller limit
- **THEN** the response is 429

#### Scenario: Text too long

- **WHEN** a caller submits text past the cap
- **THEN** the response is 400 and no model call is made

#### Scenario: Spend is attributed

- **WHEN** the endpoint makes a model call for a signed-in caller
- **THEN** the call is bound to that caller's gateway credential and tagged with the
  feature it serves

#### Scenario: Model unconfigured

- **WHEN** the deployment has no model client
- **THEN** the endpoint reports 503
