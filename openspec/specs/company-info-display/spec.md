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

### Requirement: Job page surfaces company info behind a lazily loaded tab

The job detail page SHALL render its content column behind a tab strip of exactly two
tabs, `Description` and `Company`, with `Description` selected on load. The `Description`
tab SHALL hold the content the column renders today (the model-written summary, when
present, followed by the job description).

The tab strip SHALL be rendered only when the job carries a company slug. A job without
one SHALL render its content column exactly as it did before this change, with no tab
strip and no company surface.

The `Company` tab SHALL fetch the company only when it is first activated, and SHALL
reuse that result for every later activation within the same page visit. Once loaded, it
SHALL render the company's facts card and About summary — the same components the company
detail page renders — followed by a link to that company's page.

The tab strip SHALL NOT be removed or restructured as a result of loading, whatever the
outcome: a tab that was offered before the click SHALL still be present after it.

#### Scenario: Company tab loads on first activation

- **WHEN** a visitor opens a job whose company is known and clicks the `Company` tab
- **THEN** the company is fetched, and the panel shows the company's facts card, its
  About summary, and a link to the company's page

#### Scenario: Company is fetched at most once per visit

- **WHEN** a visitor switches to the `Company` tab, back to `Description`, and to
  `Company` again
- **THEN** the company is fetched once and the second activation renders the cached
  result without a further request

#### Scenario: Job with no known company has no tab strip

- **WHEN** a visitor opens a job that carries no company slug
- **THEN** no tab strip is rendered and the content column shows the summary and
  description exactly as it does without this feature

#### Scenario: Company with no facts and no description

- **WHEN** the fetched company has neither company-info facts nor a description
- **THEN** the `Company` tab remains present and its panel states that no details are
  held for that company yet, alongside the link to the company's page

#### Scenario: Company fetch fails

- **WHEN** the request for the company fails
- **THEN** the panel reports that the company details could not be loaded and offers the
  link to the company's page, and the rest of the job page is unaffected

#### Scenario: Navigating to another job discards the previous company

- **WHEN** a visitor loads the `Company` tab for one job and then navigates client-side
  to a job at a different company
- **THEN** the panel returns to its unloaded state and shows no data from the previous
  company

### Requirement: The job page company surface is never server-rendered

Company facts and the company description SHALL NOT appear in the server-rendered HTML of
a job page. They SHALL be fetched by the browser only, and only in response to the visitor
activating the `Company` tab.

This keeps a company's summary out of the crawlable HTML of every posting that company
has open, so those pages do not compete with the company's own page for it. The job page's
server-rendered link to the company page SHALL be retained as the crawlable path between
the two.

#### Scenario: Server-rendered job page carries no company copy

- **WHEN** a job page is requested and its server-rendered HTML is inspected before any
  script runs
- **THEN** the HTML contains no company facts and no company description, and it does
  contain the link to the company's page

### Requirement: The tab strip is operable by keyboard and exposed to assistive technology

The tab strip SHALL expose the standard tab pattern: a tab list, one tab per selectable
view carrying its selected state, and a tab panel the tabs drive. The panel id each tab
references SHALL resolve to an element in the document at every point in the page's life,
including before either tab has been activated.

The content of the unselected tab SHALL remain in the document, hidden by style rather
than removed, so that switching away from the Company tab does not discard a company the
visitor has already waited for.

The left and right arrow keys SHALL move the selection between tabs.

#### Scenario: A tab's panel reference always resolves

- **WHEN** a job page with a tab strip finishes its first render, before any tab is
  clicked
- **THEN** each tab's referenced panel id resolves to an element in the document, and the
  content of the unselected tab is present but not displayed

#### Scenario: Arrow keys move between tabs

- **WHEN** a tab has keyboard focus and the visitor presses the right or left arrow key
- **THEN** the selection moves to the adjacent tab and its panel is shown

### Requirement: The job page company tab shows the employer's own links as brand marks

The Company tab SHALL render the employer's recorded outbound links — website, LinkedIn,
X, Facebook and Instagram — as a row of brand marks in that fixed order, showing only the
links that are present. Each mark SHALL carry an accessible name identifying the company
and the destination, since the mark itself carries no text.

The links SHALL open in a new tab and SHALL be marked `nofollow`. These destinations are
whatever the importer recorded and the catalogue has not vetted them; a followed link
from every posting a company has open is what a promotional submission is after.

A stored link SHALL be rendered only if its scheme is `http` or `https`. Any other value
SHALL be dropped without rendering. These values come from an external importer and are
placed in an `href`, where a `javascript:` or `data:` URL would execute script on the
site's own origin.

#### Scenario: A company with several recorded links

- **WHEN** a visitor opens the Company tab of an employer with a website, a LinkedIn page
  and an X account
- **THEN** three brand marks appear in the order website, LinkedIn, X, each naming the
  company and its destination for assistive technology, and each opening in a new tab

#### Scenario: A link with a scheme we refuse

- **WHEN** a company's stored website is a `javascript:` or `data:` URL
- **THEN** no mark is rendered for it, and the company's other valid links are unaffected

#### Scenario: A company known only by its links

- **WHEN** a company has no facts, no badges and no description, but does have a website
- **THEN** the tab renders that link rather than reporting that no details are held

### Requirement: The job page company tab shows the CEO and the office countries

The Company tab SHALL show the recorded chief executive as a fact, positioned between
Headquarters and organisation type.

The tab SHALL show the countries the company has an office in as an overlapping cluster
of flags, capped so that an employer present in dozens of countries does not overrun the
panel. Office countries SHALL NOT link to the job filter for that country: they are the
employer's sites, and a role in that country may not exist.

An office entry SHALL be shown only when it carries a two-letter ISO 3166-1 alpha-2
country code; duplicates SHALL be collapsed, keeping first-seen order.

#### Scenario: Company with a recorded CEO

- **WHEN** a visitor opens the Company tab of an employer whose CEO is recorded
- **THEN** the CEO appears in the facts row between Headquarters and Type

#### Scenario: Company present in many countries

- **WHEN** the company records more office countries than the cluster shows
- **THEN** the flags are capped and the remainder is summarised as a count

#### Scenario: Office entries that carry no usable code

- **WHEN** the recorded offices include an entry with no country code, or one whose code
  is not two letters
- **THEN** that entry is omitted and the remaining offices are shown

