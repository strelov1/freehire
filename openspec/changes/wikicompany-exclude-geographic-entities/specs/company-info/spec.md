## MODIFIED Requirements

### Requirement: A match is accepted only when confidently typed as a company or organization

A candidate Wikipedia/Wikidata match SHALL be accepted only when the matched
entity's own type classification places it within a curated set of
business/organization types (for example: company, corporation, public
company, business, bank, or a narrower subtype of one of these), AND the same
type classification does NOT also place it within a curated set of
geographic or administrative-territorial types (for example: a country, a
region, a commune, or another administrative division). A match whose type
falls outside the business/organization set, whose type cannot be
determined, or whose type also falls within the geographic/administrative
set, SHALL be rejected, and the worker SHALL make no write for that company.

The type check SHALL NOT rely on keyword-matching the entity's
natural-language description or summary text, because that approach both
rejects valid matches whose description omits a recognized keyword and
accepts invalid matches whose description happens to contain one.

#### Scenario: A same-named person, place, or concept is rejected

- **WHEN** a company's name resolves to a Wikipedia article whose subject is
  typed as a person, a place, a military unit, an abstract concept, or a
  disambiguation page
- **THEN** the worker rejects the match and writes nothing for that company

#### Scenario: A valid company match is accepted regardless of description wording

- **WHEN** a company's name resolves to an article typed as a business or
  organization, even if its description text uses wording outside any fixed
  keyword list (for example "defense contractor" or "electrical contractor")
- **THEN** the worker accepts the match and fills the company's `tagline`

#### Scenario: A same-named administrative division is rejected even if it is also typed as an organization

- **WHEN** a company's name resolves to an entity that is typed as a
  geographic or administrative-territorial division (for example a commune,
  city, or country), even if Wikidata's own multi-parent class hierarchy also
  places that entity's type under a business/organization class
- **THEN** the worker rejects the match and writes nothing for that company
