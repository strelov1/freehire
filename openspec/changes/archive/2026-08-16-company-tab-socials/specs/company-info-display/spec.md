## ADDED Requirements

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
