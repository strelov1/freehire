## ADDED Requirements

### Requirement: The contact-free projection is the one typed seam to a model

The system SHALL express "the part of a candidate's CV a model may see" as a single typed
projection of the structured résumé, and every consumer that sends CV-derived content to a model
SHALL receive that projection as a typed value — never as a serialized blob it filters, trims or
re-projects itself.

The projection's field set SHALL be a **whitelist**: a field added to the structured résumé is
withheld from every model until it is added to the projection too. The system MUST NOT enforce
this by removing known contact fields from a serialized structured résumé, because such a rule
discloses each newly-added field by default — the opposite of what the projection exists to do.

A consumer that has no projection available for the caller SHALL be told so as a distinct state
rather than inferring it from an empty serialization, so "this user has no structured résumé"
cannot be confused with "this value serialized to nothing".

#### Scenario: A newly added structured field reaches no model

- **WHEN** a field carrying personal data is added to the structured résumé and is not added to
  the contact-free projection
- **THEN** it appears in neither the fit chain's prompt nor the ATS review's prompt, with no
  change to either consumer

#### Scenario: A model-facing consumer cannot be handed the contact-bearing value

- **WHEN** a caller assembles the input for a model-facing CV consumer
- **THEN** the input's type is the contact-free projection, so passing the full structured
  résumé is not expressible

#### Scenario: Absence is a state, not an empty string

- **WHEN** the caller has no current structured résumé
- **THEN** the reader reports absence explicitly and the consumer skips the model call, rather
  than the consumer inferring absence from empty content
