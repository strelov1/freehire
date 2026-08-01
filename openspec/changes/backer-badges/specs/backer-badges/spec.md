## ADDED Requirements

### Requirement: A backer is a collection naming who selected the company

The system SHALL model a **backer** as a collection whose membership names the
accelerator or investment fund that selected the company, distinguished from an
editorial collection and from a credential by its `kind`. A backer SHALL otherwise
reuse the collection machinery unchanged — the same company-level tag set, the same
denormalization onto jobs, the same search facet, and the same landing page.

A backer SHALL state a fact about the **company**, never about the role being
viewed. Being backed by a fund says nothing about a particular posting's quality,
compensation, or hiring intent, and the presentation SHALL NOT imply otherwise.

A tag SHALL qualify as a backer only when the selecting organisation is a
identifiable brand with its own mark. Editorial themes the system curates itself —
company size, valuation, geography, sector — SHALL remain editorial collections,
because no external body selected their members.

#### Scenario: A backer reuses the collection machinery

- **WHEN** a company holds a backer tag
- **THEN** the tag appears in the company's collection set, is denormalized onto
  its jobs, is filterable through the `collections` facet, and has a landing page,
  exactly as an editorial collection does

#### Scenario: Backer, editorial and credential are distinguished by kind

- **WHEN** the collection registry is read
- **THEN** each entry's `kind` identifies it as exactly one of editorial, credential,
  or backer

### Requirement: The registry names Y Combinator, Techstars and a16z as backers

The system SHALL carry exactly four backer collections: `yc` (Y Combinator),
`techstars` (Techstars), `a16z-portfolio` (the a16z portfolio) and `a16z-speedrun`
(the a16z Speedrun accelerator cohorts).

The system SHALL keep the a16z portfolio and the Speedrun cohorts as two separate
collections rather than one. Being held in a fund's portfolio and having been
selected into its accelerator are different facts of different strength, and
merging them would present a seed-stage cohort company and a late-stage portfolio
company as carrying the same signal.

#### Scenario: A portfolio company and a cohort company are tagged differently

- **WHEN** a company appears in the a16z portfolio but not in a Speedrun cohort
- **THEN** it holds `a16z-portfolio` and not `a16z-speedrun`

#### Scenario: A remaining editorial collection gains no badge

- **WHEN** a company holds an editorial collection such as `unicorn` or `bigtech`
- **THEN** no backer badge is rendered for it

### Requirement: The a16z membership comes from the Speedrun directory, excluding its market tier

The system SHALL source both a16z collections from the Speedrun talent-network
public company directory, partitioning its entries by the directory's own `tier`
field: `a16z` yields `a16z-portfolio`, `speedrun` yields `a16z-speedrun`.

The system SHALL NOT import the directory's `market` tier under any backer tag. That
tier lists companies with no a16z relationship — large public employers among them —
and importing it would assert a fund relationship that does not exist.

The system SHALL NOT derive a16z membership from a job's ingest source. The
`speedrun` source covers only the portfolio companies that host applications on the
network itself; most are crawled through their own ATS, so source-derived membership
would understate the collection by an order of magnitude.

#### Scenario: A market-tier company earns no backer tag

- **WHEN** the directory lists a company under `tier: market`
- **THEN** it receives neither `a16z-portfolio` nor `a16z-speedrun`

#### Scenario: A portfolio company crawled through its own ATS is still tagged

- **WHEN** a directory company under `tier: a16z` has jobs ingested from a source
  other than `speedrun`
- **THEN** it holds `a16z-portfolio`

#### Scenario: The whole directory is read despite pagination

- **WHEN** the directory is fetched and reports more pages than one
- **THEN** every page is read before membership is resolved

### Requirement: A backer badge renders the brand's own mark from a committed asset

The system SHALL render a backer tag as the selecting brand's own logo, served from
an asset committed to the repository rather than resolved at runtime from a logo
service. A logo service that resolves by company name mis-resolves abbreviated
brand names onto unrelated sites, and a brand mark is the one place where a
plausible-looking wrong image is worse than no image.

The system SHALL NOT fall back to a generated monogram when a backer's mark is
unavailable. A letter tile in place of a brand mark reads as a defect; a backer tag
with no known mark SHALL render nothing at all.

The badge SHALL carry an accessible name stating which brand backs the company, and
SHALL NOT rely on hover alone to convey it.

#### Scenario: An unknown backer slug renders nothing

- **WHEN** a company holds a backer tag the presentation layer has no mark for
- **THEN** no badge is rendered, and no placeholder or monogram appears in its place

#### Scenario: The badge is announced to assistive technology

- **WHEN** a backer badge is rendered
- **THEN** its accessible name names the backing brand

### Requirement: Backer badges appear beside the company across job and company surfaces

The system SHALL render a company's backer badges adjacent to the company's name on
the job feed card, the job page, the company page, and the company list, so the
badge reads as a fact about the employer.

The system SHALL NOT place backer badges in the job card's signal row, which carries
role-level facts — the reality chip, facet chips, credential chips and the country
flag stack. A backer is a company-level fact, and a fifth chip type would displace
role information the card exists to convey.

The system SHALL render a backer's mark on its `/collections` hub card, on its
collection landing header, and inside its collection filter chip.

#### Scenario: The job card shows the badge with the company, not in the signal row

- **WHEN** a job whose company holds a backer tag is rendered in the feed
- **THEN** the badge appears beside the company name, and the signal row is unchanged

#### Scenario: The collection filter chip carries the mark

- **WHEN** the collections facet is rendered as filter chips
- **THEN** a backer collection's chip shows its mark alongside its label
