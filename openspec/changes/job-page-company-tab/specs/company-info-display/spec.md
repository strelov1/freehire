## ADDED Requirements

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
