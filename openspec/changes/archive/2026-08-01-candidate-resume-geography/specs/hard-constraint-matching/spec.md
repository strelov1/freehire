## ADDED Requirements

### Requirement: Country evidence may fall back to the candidate's derived geography

The assembly of the evaluator's CV evidence SHALL source the candidate's country from
their asserted profile base country when they have stated one, and MAY fall back to the
country derived from their CV when they have not. A derived geography naming more than
one country MUST yield no country rather than one chosen arbitrarily, so the fallback
can never manufacture evidence the candidate did not supply.

This changes only which category can be evaluated, not the evaluator: the pure evaluator
continues to skip a category whose evidence field is empty, and continues to emit no
blocker where it has nothing to compare. The fallback exists because the two constraints
that read this field — work-authorization and location-and-work-mode — were silently
inert for the large majority of profiles, which state no base country at all.

#### Scenario: A stated base country is used unchanged

- **WHEN** evidence is assembled for a caller whose profile states a base country
- **THEN** that country is the evidence country, regardless of what their CV derives

#### Scenario: A derived country fills an unstated base country

- **WHEN** evidence is assembled for a caller who states no base country and whose CV
  derives exactly one country
- **THEN** that derived country is the evidence country, and the work-authorization and
  location categories become evaluable

#### Scenario: An ambiguous derived geography supplies no country

- **WHEN** evidence is assembled for a caller who states no base country and whose CV
  derives more than one country
- **THEN** the evidence country is empty and both country-dependent categories are skipped
  silently, exactly as when no geography is known at all

#### Scenario: A caller with neither source is unaffected

- **WHEN** evidence is assembled for a caller who states no base country and has no
  derived geography
- **THEN** the evidence country is empty and no blocker is emitted for either category
