## ADDED Requirements

### Requirement: A discipline phrase does not corroborate an ambiguous concept

The skill matcher gates a set of ambiguous single-word canonicals (`ai`,
`automation`, `seo`, `crm`, …) that tag only when the same text carries a strong,
concrete token. A marketing **discipline** named as a phrase ("content marketing",
"marketing automation", "demand generation") is a concept, not a concrete
technology, and SHALL NOT act as that corroborator — otherwise the "AI-powered"
prose that saturates marketing postings would tag the whole population with `ai`.
A named **product** ("Ahrefs", "Klaviyo", "Semrush") SHALL keep corroborating, as
it always has. A discipline phrase SHALL still emit its own canonical, and SHALL
itself survive without corroboration.

#### Scenario: A discipline does not pull in the gated concept

- **WHEN** a posting says "AI-powered marketing automation for our sales team"
- **THEN** the emitted skills include the marketing-automation canonical but
  neither `ai` nor `automation`

#### Scenario: A product still corroborates

- **WHEN** a posting says "SEO Specialist — keyword research in Ahrefs and Semrush"
- **THEN** the emitted skills include `ahrefs`, `semrush` and the gated `seo`

#### Scenario: A discipline stands alone

- **WHEN** a posting names a marketing discipline and nothing else the dictionary
  knows
- **THEN** that discipline's canonical is still emitted
