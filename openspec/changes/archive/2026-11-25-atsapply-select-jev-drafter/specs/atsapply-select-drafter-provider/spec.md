## ADDED Requirements

### Requirement: Jev answers select-kind draftable fields when configured

The system SHALL answer a `draftable` field whose `Kind` is `select` using the Typesafe AI
Jev `Choice` question type when `TYPESAFE_API_KEY` is configured, instead of the
generative `LLMDrafter.Draft` call it answers with by default. The answer SHALL be either
a value copied verbatim from one of the field's offered option labels, or `ok=false` when
nothing stated supports any option or the question falls in a categorically excluded
topic — the same contract `LLMDrafter.Draft` guarantees today. `text`/`textarea` fields
SHALL NOT be affected by this requirement; they continue to answer through the LLM
regardless of this setting.

#### Scenario: A clear match picks the supported option

- **WHEN** a `select` field is offered whose options include one the candidate's stated
  facts clearly support
- **THEN** the returned answer is that option's label, copied verbatim from the offered
  list, and `ok` is true

#### Scenario: Nothing stated supports any option

- **WHEN** a `select` field is offered and no stated fact supports any of its options
- **THEN** `ok` is false and no option is guessed or invented

#### Scenario: A categorically excluded topic declines even with a superficially supportive fact

- **WHEN** a `select` field asks about identity, demographics, compensation, or legal work
  status, and a stated fact could be read as superficially suggesting an answer (e.g. a
  past salary-negotiation figure against a compensation question, or stated on-site work
  history in a country against that country's legal-authorization question)
- **THEN** `ok` is false — the categorical exclusion applies regardless of what is stated

#### Scenario: text/textarea fields are unaffected

- **WHEN** a `text` or `textarea` draftable field is offered, with `TYPESAFE_API_KEY`
  configured
- **THEN** the answer is produced by `LLMDrafter.Draft` exactly as before this change,
  regardless of the Jev configuration

### Requirement: A Jev failure falls back to the LLM drafter, never to an unanswered field

The system SHALL retry a `select` field's draft through the wrapped `LLMDrafter.Draft`
whenever the Jev-backed path fails for any reason (network error, authentication failure,
a response that cannot be parsed as the expected answer shape). The caller SHALL NOT
observe a Jev failure as an error or as a missing answer when the LLM fallback can answer
the question.

#### Scenario: Jev is unreachable

- **WHEN** `TYPESAFE_API_KEY` is configured and the Jev API call fails (timeout, network
  error, non-2xx response)
- **THEN** the field's draft retries through the wrapped `LLMDrafter.Draft` and returns its
  answer, with no error surfaced to the caller on account of the Jev failure alone

#### Scenario: Jev returns an unparseable response

- **WHEN** `TYPESAFE_API_KEY` is configured and Jev's response cannot be read as a valid
  choice answer
- **THEN** the draft falls back to `LLMDrafter.Draft` exactly as it would for a network
  failure

### Requirement: Jev is never called when unconfigured

The system SHALL NOT call the Jev API for a `select` field's draft when `TYPESAFE_API_KEY`
is unset or empty. In that state, drafting SHALL be identical to `LLMDrafter.Draft` alone
for every field kind — the same calls, the same answers, the same errors.

#### Scenario: No Typesafe key configured

- **WHEN** `TYPESAFE_API_KEY` is unset
- **THEN** every field's draft is answered by `LLMDrafter.Draft` directly, and no request
  is made to the Jev API
