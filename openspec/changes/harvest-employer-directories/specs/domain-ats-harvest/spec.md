## ADDED Requirements

### Requirement: Unmatched-company extraction from a job board's employer directory

The system SHALL provide an extraction step that reads a regional job board's
**employer directory** — the board's own list of the companies posting on it — and
emits the same `{name, website}` worklist the collection-dataset extraction emits, so
the resolve step consumes it without knowing where it came from.

The step SHALL emit only companies whose normalized-name slug is **absent** from a
supplied set of existing company slugs, and SHALL omit a company with no website,
matching the collection-dataset step's rules — there is nothing for resolve to follow
without a website, and an employer already in the catalogue is not a candidate.

A directory SHALL be expressed as two operations: listing the directory's company
pages, and parsing one page into a name and a website. Parsing SHALL be a pure
function over the page's bytes, so a board's parser is testable without network
access. Listing MAY page, and the run SHALL report progress, because a directory is
many requests rather than one dataset download.

A page from which no website can be read SHALL be skipped, never abort the run: a
directory mixes employers that publish a website with those that do not, and the
latter are the expected case rather than a fault.

A run in which **no** page yields a site SHALL fail with a non-zero exit rather than
emit an empty worklist. An empty worklist is indistinguishable from "the directory
holds no new employers", and the likeliest cause is that the board changed the shape
the parser reads.

Committing the boards this worklist eventually produces remains governed by the
board-harvest validation rules: every candidate is probed live against its platform's
own API and kept only if that platform reports open jobs for it.

#### Scenario: A company page carrying a website is emitted

- **WHEN** a directory company page states the employer's own website and the
  employer's slug is absent from the supplied company-slug set
- **THEN** the step emits that company's name and website

#### Scenario: A company page with no website is skipped

- **WHEN** a directory company page states no website
- **THEN** that company is not emitted, and the run continues to the next page

#### Scenario: A company already in the catalogue is dropped

- **WHEN** a directory company's normalized-name slug is present in the supplied
  company-slug set
- **THEN** it is not emitted, even though its page carries a website

#### Scenario: Two directory entries for one website collapse

- **WHEN** two directory company pages state the same website
- **THEN** the step emits one company for that website

#### Scenario: An unreadable page does not abort the run

- **WHEN** a company page is truncated, carries no parseable payload, or is missing
  the field the website is read from
- **THEN** that page yields no company and the run continues

#### Scenario: A run that yields nothing fails loudly

- **WHEN** every page in a directory run yields no company
- **THEN** the run exits non-zero and emits no worklist

#### Scenario: The worklist is interchangeable with the dataset worklist

- **WHEN** the directory step's output is passed to the resolve step
- **THEN** resolve consumes it exactly as it consumes the collection-dataset
  extraction's output, with no directory-specific handling
