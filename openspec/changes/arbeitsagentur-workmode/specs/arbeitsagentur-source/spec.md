## MODIFIED Requirements

### Requirement: Arbeitsagentur maps a first-party posting to a Job with a scraped description

For each kept posting the adapter SHALL fetch the server-rendered detail page
`https://www.arbeitsagentur.de/jobsuche/jobdetail/<refnr>` and map the posting to a normalized `Job`
carrying `refnr` as its `ExternalID`, the detail page URL as its canonical URL, `titel` as its title,
`arbeitgeber` as its company, the `arbeitsort` (`ort`, `region`, `land`) as its location,
`aktuelleVeroeffentlichungsdatum` as its posted-at, and the detail page's `Stellenbeschreibung` (or,
when that block is absent, the page's meta-description summary) as its sanitized description.

The adapter SHALL also read the detail page's `jobdetail.homeofficemoeglich` boolean from the same
`ng-state` payload and set the `Job`'s remote flag and work mode from it: `true` marks the job remote
with work mode `remote`; `false` or absent leaves both unset. No additional request is made — the
flag is taken from the detail fetch that already supplies the description.

A detail-page fetch that fails or yields no description SHALL NOT drop the posting — it is emitted with
an empty description (and no remote flag) — and a single failed page SHALL NOT abort the crawl.

#### Scenario: A first-party posting maps to a job

- **WHEN** the adapter keeps a first-party posting and fetches its detail page
- **THEN** it yields one `Job` with `ExternalID` set to `refnr`, the jobdetail page as the URL, the
  title, company `arbeitgeber`, the `arbeitsort` location, the publish date, and the scraped
  `Stellenbeschreibung` as the description

#### Scenario: A home-office posting is marked remote

- **WHEN** a kept posting's detail page reports `homeofficemoeglich: true`
- **THEN** the mapped `Job` has `Remote: true` and work mode `remote`

#### Scenario: A non-home-office posting is not marked remote

- **WHEN** a kept posting's detail page reports `homeofficemoeglich: false` or omits it
- **THEN** the mapped `Job` leaves the remote flag and work mode unset

#### Scenario: A posting whose detail page yields no description is still emitted

- **WHEN** a kept posting's detail page fetch fails or carries no description block
- **THEN** the adapter still yields the `Job` (with an empty description and no remote flag) and
  continues the crawl
