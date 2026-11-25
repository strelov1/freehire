## ADDED Requirements

### Requirement: Jev answers combobox Choose questions when configured

The system SHALL answer `autofillagent.Planner.Choose` (one question, a closed set of
options read live from the page, plus the candidate's profile) using the Typesafe AI Jev
`Choice` question type when `TYPESAFE_API_KEY` is configured, instead of the generative LLM
call it answers with by default. The answer SHALL be either a value copied verbatim from
the offered `options`, or empty (`""`) when nothing in the profile supports any option —
the same contract `LLMPlanner.Choose` guarantees today. `Plan` SHALL NOT be affected by this
requirement; it continues to answer through the LLM regardless of this setting.

#### Scenario: A clear match picks the supported option

- **WHEN** `Choose` is asked a question whose options include one the candidate's profile
  clearly supports (e.g. the profile states a location and one option names that location)
- **THEN** the returned value is that option, copied verbatim from the offered list

#### Scenario: Nothing in the profile supports any option

- **WHEN** `Choose` is asked a question and no field in the candidate's profile supports
  any of the offered options
- **THEN** the returned value is empty (`""`), and no option is guessed or invented

#### Scenario: Plan is unaffected

- **WHEN** `Plan` is called to map the whole form, with `TYPESAFE_API_KEY` configured
- **THEN** `Plan`'s answer is produced by the LLM exactly as before this change, regardless
  of the Jev configuration

### Requirement: A Jev failure falls back to the LLM, never to an unanswered field

The system SHALL retry a `Choose` call through `LLMPlanner.Choose` whenever the Jev-backed
path fails for any reason (network error, authentication failure, a response that cannot be
parsed as the expected answer shape). The caller SHALL NOT observe a Jev failure as an
error or as a missing answer when the LLM fallback can answer the question.

#### Scenario: Jev is unreachable

- **WHEN** `TYPESAFE_API_KEY` is configured and the Jev API call fails (timeout, network
  error, non-2xx response)
- **THEN** `Choose` retries the same question through `LLMPlanner.Choose` and returns its
  answer, with no error surfaced to the caller on account of the Jev failure alone

#### Scenario: Jev returns an unparseable response

- **WHEN** `TYPESAFE_API_KEY` is configured and Jev's response cannot be read as a valid
  choice answer
- **THEN** `Choose` falls back to `LLMPlanner.Choose` exactly as it would for a network
  failure

### Requirement: Jev is never called when unconfigured

The system SHALL NOT call the Jev API from `Choose` when `TYPESAFE_API_KEY` is unset or
empty. In that state, `Choose`'s behavior SHALL be identical to `LLMPlanner.Choose` alone —
the same calls, the same answers, the same errors.

#### Scenario: No Typesafe key configured

- **WHEN** `TYPESAFE_API_KEY` is unset
- **THEN** every `Choose` call is answered by `LLMPlanner.Choose` directly, and no request
  is made to the Jev API
