## ADDED Requirements

### Requirement: Role × country rollup

The rollup worker SHALL precompute, for every category and every country, the number
of open postings now and at the start of the growth window, together with an
all-countries bucket per category. It SHALL count canonical, public, categorized
postings — `duplicate_of IS NULL`, not private, non-empty `category` — with or
without a resolved seniority, so the grid covers the catalogue rather than the
seniority-resolved subset the role rollup is keyed on. A posting SHALL contribute
once to each of its countries and exactly once to the all-countries bucket; a
posting with no country SHALL contribute to the all-countries bucket only. It SHALL
be rebuilt in the same atomic swap as the other insights rollups.

#### Scenario: A posting in two countries

- **WHEN** an open canonical `backend` posting lists `de` and `nl`
- **THEN** it adds one to `(backend, de)`, one to `(backend, nl)`, and one — not
  two — to `(backend, all countries)`

#### Scenario: Duplicates and private postings are excluded

- **WHEN** an open posting carries a `duplicate_of` marker, or is private
- **THEN** it contributes to no cell

#### Scenario: Seniority is not required

- **WHEN** an open canonical posting has a category but no resolved seniority
- **THEN** it is counted in its category's cells

### Requirement: Role × country endpoint

The system SHALL expose a public, unauthenticated
`GET /api/v1/insights/role-countries` endpoint returning the grid's cells from the
rollup. It SHALL choose the columns itself: the countries with the most open
postings in the rollup, in descending order, their number set by an optional
`countries` parameter (default 10, capped at a fixed maximum, `400` beyond it or
when not a positive integer). It SHALL respond with
`{"data": [...], "meta": {...}}`, where each `data` row carries `category`,
`country` (empty string for the all-countries bucket), `open_count` and
`open_count_prev`, and `meta` carries the ordered `countries` list and the growth
window in days. Every category with an open posting SHALL appear in the
all-countries bucket. The response SHALL be aggregate-only.

#### Scenario: Default grid

- **WHEN** a client requests `GET /api/v1/insights/role-countries`
- **THEN** the response is `200`, `meta.countries` lists the 10 countries with the
  most open postings in descending order, and `data` holds the all-countries cell of
  every category plus each category's cell in each of those countries that has one

#### Scenario: Fewer columns

- **WHEN** a client requests `GET /api/v1/insights/role-countries?countries=3`
- **THEN** `meta.countries` lists exactly the top 3 countries and no `data` row
  names any other country

#### Scenario: Out-of-range column count rejected

- **WHEN** a client requests `countries=0`, a value above the maximum, or a
  non-number
- **THEN** the response is `400` and no data is returned

### Requirement: Role × country grid on the insights hub

The `/insights` page SHALL render the grid above the category cards, server-side, as
a table: categories as rows ordered by all-countries open count, an all-countries
column first and then the endpoint's countries in its order. It SHALL show the 15
largest categories and offer to show all of them.

Every cell SHALL be a link. A cell whose category has a role landing and whose open
count is at least TWICE the role landing's publication floor (`MIN_PAIR_OPEN`) SHALL
link to that `(category, country)` role landing; any other cell SHALL link to the job
search filtered to that category and country. The margin exists because the landing
gates on the live search count while the grid reads the rollup, and the two differ:
a cell linked at the bare floor could point at a page that answers 404. An
all-countries cell SHALL link to the category's country table (`/roles/<category>`)
when the category has a landing, otherwise to the job search for the category. A row
heading SHALL link to that category's insights page and a column heading to the job
search for that country.

#### Scenario: A large cell opens the role landing

- **WHEN** a visitor activates the `healthcare` × `de` cell and it holds at least
  twice `MIN_PAIR_OPEN` postings
- **THEN** they land on the healthcare role landing for Germany

#### Scenario: A small cell opens the search

- **WHEN** a cell holds fewer than twice `MIN_PAIR_OPEN` postings
- **THEN** it links to `/jobs` filtered to its category and country, never to a role
  landing

#### Scenario: The table works without JavaScript

- **WHEN** the page is fetched without executing JavaScript
- **THEN** the HTML contains the table with its counts and its links

#### Scenario: Rows beyond the first 15

- **WHEN** a visitor asks to see all categories
- **THEN** every category in the rollup appears as a row

### Requirement: Open-count and growth views

The grid SHALL offer two views of the same cells. The open-count view SHALL show each
cell's open count and SHALL shade it by the cell's share of its COLUMN total — the
category's share of that country's postings — so columns of very different sizes are
comparable and a country's specialisation is visible. The growth view SHALL show
`(open_count − open_count_prev) / open_count_prev` as a signed percentage, shaded
on a scale that diverges from a neutral zero, and SHALL show no percentage (a
neutral "—") when `open_count_prev` is below a published floor, because a tiny base
turns noise into a headline. Neither view SHALL rely on colour alone: the number is
always printed. The page SHALL state that growth compares postings by their
publication date and describes freehire's catalogue, not the whole labour market.

#### Scenario: Shading is relative to the country

- **WHEN** `software_engineering` is 9% of India's open postings and 5% of the
  United States', though the United States cell holds more postings
- **THEN** the India cell is shaded darker in the open-count view

#### Scenario: Small base shows no percentage

- **WHEN** a cell's `open_count_prev` is below the floor
- **THEN** the growth view shows "—" for it rather than a percentage

#### Scenario: Decline is distinguishable without colour

- **WHEN** a cell's open count fell over the window
- **THEN** the growth view prints a negative percentage with a down marker
