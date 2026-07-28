# llm-structured-outputs Specification

## Purpose
How a model's JSON response is constrained by a schema derived from the Go contract
it will be decoded into: where that schema comes from, how controlled vocabularies
reach it, what a call guarantees, and what still holds when a provider stops honouring
it. The schema is a first line, never a proof — every sanitiser and validator on the
receiving side stays where it was.

## Requirements

### Requirement: A contract type is the single source of its request schema

The system SHALL derive the JSON Schema sent to the model from the Go contract type
by reflection, never from a hand-written copy. The derived schema SHALL carry every
JSON-tagged exported field of the type, name each field by its JSON tag, and be
post-processed for strict mode: `additionalProperties: false` on every object, every
field listed in `required`, and each field the contract marks `omitempty` widened to
admit `null`. Unexported fields and fields tagged `json:"-"` SHALL be absent from the
schema.

#### Scenario: Schema matches the contract type

- **WHEN** a schema is derived from a contract struct
- **THEN** its property names are exactly the JSON tags of the type's exported fields, with no property the type does not have and none of the type's fields missing

#### Scenario: A new field reaches the model without a second edit

- **WHEN** a field is added to a contract type
- **THEN** the derived schema contains it on the next call, with no separate schema file to update

#### Scenario: Optional fields admit null rather than absence

- **WHEN** a contract field is tagged `omitempty`
- **THEN** the derived schema lists it in `required` and widens its type to admit `null`, because strict mode permits no absent key

#### Scenario: Unexported and skipped fields are not requested

- **WHEN** a contract type carries an unexported field or one tagged `json:"-"`
- **THEN** neither appears in the derived schema

### Requirement: Controlled vocabularies are attached at the call site

The system SHALL let a caller constrain a schema field to a controlled vocabulary by
an explicit override supplied where the schema is built, keeping the vocabularies in
`internal/vocab` as their single definition. A struct tag SHALL NOT be used to carry
enum values, so a contract package never restates a list `internal/vocab` owns.

#### Scenario: An enum field is constrained to its vocabulary

- **WHEN** a schema is built with an override naming a field and a vocabulary
- **THEN** the schema's property for that field carries the vocabulary as its `enum` and every other property is unchanged

#### Scenario: An override names a field the type does not have

- **WHEN** a schema is built with an override for a field absent from the contract type
- **THEN** the build fails loudly rather than producing a schema silently missing the constraint

### Requirement: Schema-constrained generation is opt-in per call

The system SHALL accept an optional schema on each JSON generation call, leaving the
existing unconstrained behaviour as the default. A call given a schema SHALL send it
as the request's response format in strict mode; a call given none SHALL behave
exactly as before. Because the underlying client binds the response format to the
client rather than the call, the system SHALL build and reuse one model per distinct
schema, so repeated calls with the same schema construct nothing new.

#### Scenario: A call without a schema is unchanged

- **WHEN** a caller invokes JSON generation with no schema option
- **THEN** the request carries the plain JSON mode it carries today and no schema is sent

#### Scenario: A call with a schema constrains the response

- **WHEN** a caller invokes JSON generation with a schema
- **THEN** the request carries that schema as a strict `json_schema` response format under the given name

#### Scenario: Repeated calls with one schema reuse one model

- **WHEN** the same schema is used for several calls on one client
- **THEN** the underlying model is constructed once and reused, and calls with a different schema do not disturb it

#### Scenario: Streaming generation accepts a schema too

- **WHEN** a caller streams a JSON generation with a schema
- **THEN** the schema constrains the response exactly as in the non-streaming path, and thinking tokens are still forwarded

### Requirement: The response is validated as if the schema were ignored

The system SHALL keep every existing guard on a model response — the persist-time
sanitiser, the vocabulary validation, and the code-fence stripper — on the
schema-constrained path. A provider that stops honouring a schema returns an
ordinary success, so the system MUST NOT treat a schema as proof of the response's
shape.

#### Scenario: A provider silently ignores the schema

- **WHEN** a response arrives with a value outside the vocabulary despite the schema
- **THEN** the existing validation rejects that value exactly as it does today, and the surrounding fields are kept

#### Scenario: Bounds are still applied

- **WHEN** a schema-constrained response carries an over-long string or an oversized array
- **THEN** the sanitiser clips and caps them as it does on the unconstrained path, because a schema bounds neither

### Requirement: Truncation of fractional years survives the migration

The system SHALL continue to truncate a fractional years-of-experience figure rather
than round it, and SHALL request that field as a string so the decision stays on the
receiving side. Constrained decoding coerces the type but rounds the value, which
would inflate a candidate's stated experience.

#### Scenario: A fractional total is truncated, not rounded

- **WHEN** a model reports a total of 5.9 years
- **THEN** the stored value is 5

#### Scenario: The schema does not ask for an integer there

- **WHEN** the structured-CV schema is derived
- **THEN** its total-years property is a string, leaving the arithmetic to the existing decoder
