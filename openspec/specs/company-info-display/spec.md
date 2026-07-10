# company-info-display Specification

## Purpose
TBD - created by archiving change company-info-display. Update Purpose after archive.
## Requirements
### Requirement: Company page surfaces authoritative company facts

The company detail page SHALL render the company's authoritative company-info facts in a
card between the page header and the jobs list. The card SHALL render only the fields that
are present, and SHALL render nothing at all when the company has no company-info, so an
unenriched company shows no empty container.

When present, the card SHALL show: the tagline; an inline facts line composed of founding
year, employee count, HQ country (the ISO code resolved to an English country name), and
organization type, with only the present facts shown; the industries as chips; and a website
link that opens in a new tab. When present in the `company_info` payload, the card SHALL also
show funding (type, amount, year), stock listing (exchange and symbol), the parent company,
and subsidiaries.

#### Scenario: Enriched company shows its facts

- **WHEN** a visitor opens the page of a company that has company-info
- **THEN** the card appears above the jobs list showing the present facts (tagline, founded,
  employees, HQ country name, organization type, industries, website) and omitting any facts
  that are absent

#### Scenario: Unenriched company shows no card

- **WHEN** a visitor opens the page of a company that has no company-info fields
- **THEN** no company-info card is rendered and the page shows the header and jobs list as
  before

#### Scenario: HQ country code is shown as a country name

- **WHEN** the company's HQ country is stored as an ISO 3166-1 alpha-2 code (e.g. `US`)
- **THEN** the card displays the resolved English country name (e.g. "United States")

#### Scenario: Rare facts appear only when present

- **WHEN** the company's `company_info` payload contains funding, stock, parent, or
  subsidiaries data
- **THEN** those sections are shown; and when a company lacks them, those sections are omitted

#### Scenario: Reference company page

- **WHEN** a visitor opens the page of a reference company that has company-info but no jobs
- **THEN** the card is shown as the page's main content and the jobs list shows its empty
  state below

