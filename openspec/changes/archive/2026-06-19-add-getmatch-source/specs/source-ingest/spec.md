## ADDED Requirements

### Requirement: getmatch is a registered boardless aggregator provider

The system SHALL register a `getmatch` adapter so the getmatch.ru IT job marketplace can be
listed in a board file as a boardless aggregator (its config entry carries no `board`, and the
provider remains in the source facet). The adapter SHALL enumerate the marketplace from the
public, keyless feed `GET https://getmatch.ru/api/offers`, paginating with `offset`/`limit` and
stopping when `offset` reaches the response's `meta.total`, bounded by a maximum page count so a
missing or invalid total cannot loop. Each offer's employer SHALL come from the offer's own
`company.name` (not the configured company, which is a validation placeholder). The adapter
SHALL yield the normalized job shape with `external_id` set to the offer's numeric `id`, `url`
set to `https://getmatch.ru` joined with the offer's relative `url`, `title` set to `position`,
`location` set to the offer's distinct `location_items[].label` values joined, and `posted_at`
parsed from `published_at`. The adapter SHALL include all `offer_type` values (both regular
vacancies and one-day-offer event cards).

#### Scenario: The marketplace is enumerated from the paginated public feed

- **WHEN** a board file lists a boardless entry with provider `getmatch`
- **THEN** the adapter requests `https://getmatch.ru/api/offers` with increasing `offset`,
  stops once `offset` reaches `meta.total`, and yields each offer as the normalized job shape
  with `external_id` set to the offer `id`, `url` set to the absolute offer URL, `company` set
  to the offer's own `company.name`, and `posted_at` parsed from `published_at`

#### Scenario: A failed first page is a board error; a failed later page ends enumeration

- **WHEN** the first `/api/offers` page request fails
- **THEN** the adapter returns an error for the board
- **WHEN** a later page request fails after at least one page succeeded
- **THEN** the adapter stops paginating and returns the jobs gathered so far without error

### Requirement: getmatch descriptions come from the per-offer detail endpoint

The adapter SHALL fetch each offer's full HTML description from the detail endpoint
`GET https://getmatch.ru/api/offers/{id}` and yield it as the job `description` after HTML
sanitization. When the detail `description` is empty (for example one-day-offer event cards) or
the detail request fails, the adapter SHALL fall back to the list offer's short
`offer_description` rather than dropping the offer.

#### Scenario: Full description is taken from the detail endpoint and sanitized

- **WHEN** an offer's detail endpoint returns a non-empty HTML `description`
- **THEN** the adapter yields that HTML as the job `description`, sanitized (active content
  stripped, structure such as paragraphs and lists kept)

#### Scenario: Empty or failed detail falls back to the list summary

- **WHEN** an offer's detail `description` is empty or the detail request fails
- **THEN** the adapter yields the list offer's `offer_description` as the job `description` and
  still yields the offer

### Requirement: getmatch derives a structured work mode from per-location formats

The adapter SHALL derive the structured work mode from the offer's `location_items[].format`,
mapping `remote` to `remote`, `hybrid` to `hybrid`, and `office` to `onsite`, and ignoring the
`relocation_company` and `relocation_candidate` flags (which denote relocation support, not a
work mode). The adapter SHALL set the structured work mode only when the offer's locations
resolve to a single distinct work mode; when they resolve to more than one (or none), the
structured work mode SHALL be left empty so the pipeline's location parser decides. The job
SHALL be marked remote when and only when the derived work mode is `remote`.

#### Scenario: A single work mode is emitted as the structured work mode

- **WHEN** every work-mode-bearing `location_items` entry of an offer has the same `format`
  (e.g. all `office`)
- **THEN** the adapter sets the structured work mode to the mapped value (`onsite`), and marks
  the job remote only when that value is `remote`

#### Scenario: Mixed work modes leave the structured field empty

- **WHEN** an offer's `location_items` resolve to more than one distinct work mode (e.g. one
  `remote` and one `office`)
- **THEN** the adapter leaves the structured work mode empty, so the pipeline falls back to
  parsing the location string

#### Scenario: Relocation flags do not count as a work mode

- **WHEN** an offer's only non-`relocation_*` format is a single work mode and the remaining
  entries are `relocation_company`/`relocation_candidate`
- **THEN** the relocation entries are ignored and the single work mode is emitted as the
  structured work mode
