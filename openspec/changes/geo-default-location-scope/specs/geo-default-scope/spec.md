## ADDED Requirements

### Requirement: Derive the visitor's region from the edge country header

The system SHALL read the visitor's ISO 3166-1 alpha-2 country from the
`CF-IPCountry` request header on the server and map it to a controlled region value
using the existing country→region grouping (`COUNTRY_REGION_MAP`). No other
geolocation source SHALL be consulted — no GeoIP database, no third-party lookup,
no browser geolocation prompt.

The derived region SHALL be served from a dedicated endpoint marked
`Cache-Control: private, no-store`, and SHALL NOT travel in any page's server-render
payload. Server-rendered data is embedded in the document, so a region delivered
that way would make the HTML differ by country — the exact thing this design exists
to avoid.

The header is advisory and often absent: a direct origin request, local
development, and a Cloudflare zone with IP Geolocation switched off all omit it.
Cloudflare also sends the reserved value `XX` when it cannot place the address, and
`T1` for Tor exits. Each of these SHALL leave the visitor with no derived region
rather than a guessed one.

#### Scenario: A placeable country maps to a region

- **WHEN** the endpoint is called on a request carrying `CF-IPCountry: BR`
- **THEN** it answers with the region `COUNTRY_REGION_MAP` groups `BR` under

#### Scenario: The region never rides along with a page

- **WHEN** any page is server-rendered
- **THEN** its embedded render payload carries no country and no derived region

#### Scenario: The endpoint is not cached

- **WHEN** the endpoint answers
- **THEN** the response forbids storing, so no shared cache can hand one visitor's region to another

#### Scenario: The header is absent

- **WHEN** a request arrives with no `CF-IPCountry` header
- **THEN** no region is derived, no error is raised, and the visitor is treated exactly as one whose country is unknown

#### Scenario: The edge cannot place the address

- **WHEN** the header carries `XX`, `T1`, or a value the country→region grouping does not contain
- **THEN** no region is derived

### Requirement: Apply the derived scope only on the client

The system SHALL NOT let the derived country change the server-rendered response
in any way — not the rendered HTML, not the data the server loads, not a redirect.
The scope SHALL be applied after hydration, on the client only.

This is a caching requirement, not a stylistic one: the jobs feed is cached at the
edge by URL, and a response that varied by country would need either a `Vary` on the
header or the country in the cache key, multiplying every cached entry by the number
of countries that reach it.

#### Scenario: Two countries receive the same document

- **WHEN** two visitors in different countries request the same jobs-feed URL
- **THEN** both receive byte-identical server-rendered HTML, and the difference appears only after hydration

#### Scenario: A cache entry serves any country

- **WHEN** an edge-cached jobs-feed response is served to a visitor from a country other than the one whose request populated the cache
- **THEN** the response is correct for that visitor, because it carries no country-specific content

### Requirement: Automated clients are never scoped

The system SHALL NOT apply the derived scope to a crawler, and the endpoint SHALL
answer a recognized automated client with no region.

Most of this site's traffic is automated. A renderer that runs the page's JavaScript
would otherwise index a feed scoped to whatever country its exit address sits in,
which is a scope no human asked for and no canonical URL describes. Being a crawler
is not something the client is asked about — the endpoint decides from the request
it is already serving.

#### Scenario: A rendering crawler asks for a region

- **WHEN** a request whose user agent identifies a known crawler reaches the endpoint
- **THEN** it answers with no region, and the crawler's feed stays unscoped

#### Scenario: A non-rendering crawler

- **WHEN** a crawler that does not execute JavaScript fetches the feed
- **THEN** it never reaches the endpoint at all, and receives the same unscoped document any first-time visitor's server render produces

### Requirement: The guess must not cost the feed its page-experience metrics

The derived scope replaces a rendered list with a different one after hydration.
The system SHALL NOT let that swap shift the surrounding layout, and SHALL NOT let
it become the feed's largest contentful paint.

Both are ranking inputs measured from real Chrome users, not from crawlers, so
excluding crawlers from the scope does not exclude the cost. The swap happens before
any user input, which is precisely the window where a late paint replaces the
largest-paint candidate and where a shift is attributed in full.

The gate is measurement, not argument: if the feed's measured performance or
layout-shift figures regress, the derived scope SHALL be shipped in its suggestion
form — offered and applied on a click — rather than applied automatically.

#### Scenario: The swap does not move the page

- **WHEN** the scoped list replaces the unscoped one
- **THEN** the surrounding layout does not move, and no layout shift is attributed to the swap

#### Scenario: The scheduled watchdog exercises this path

- **WHEN** the scheduled Lighthouse run loads the feed, which it does with empty browser storage and therefore always through the derived-scope branch
- **THEN** its recorded scores stay above the floors configured for the feed

#### Scenario: Measurement finds a regression

- **WHEN** before-and-after measurement of the feed shows the performance score or the layout-shift figure worse than the baseline
- **THEN** the automatic application is abandoned in favour of the suggestion form, and the regression is not shipped

### Requirement: The derived scope is the lowest-priority source of filter state

The system SHALL apply the derived scope to the standalone jobs feed only when no
other source has supplied geography: no geography params in the URL, and no stored
filter set being restored. When the derived scope applies, it SHALL set the
`regions` facet to the visitor's region **and** the worldwide region, so a remote
role open to everyone is never hidden by a regional default.

The derived scope SHALL NOT be written to `hire.jobFilters`. Storage records what
the visitor chose; a guess is not a choice.

#### Scenario: URL geography wins

- **WHEN** a visitor opens `/jobs?regions=eu` from a country that would derive `latam`
- **THEN** the URL's `eu` is applied and the derived scope is not

#### Scenario: A stored filter set wins

- **WHEN** a client-side navigation to a bare `/jobs` restores a stored filter set
- **THEN** the restored set is applied and the derived scope is not, whether or not the stored set names any geography

#### Scenario: Nothing else supplied geography

- **WHEN** a visitor from Brazil lands on a bare `/jobs` with no stored filters and the derived region is `latam`
- **THEN** the `regions` facet is set to `latam` and the worldwide region, the URL is rewritten to reflect it, and the list reloads scoped

#### Scenario: A stored set without geography still wins

- **WHEN** a stored filter set holds only `seniority=senior` and is restored on a bare `/jobs`
- **THEN** the derived scope is not applied, and the visitor sees senior roles worldwide

### Requirement: The guess is offered once per browser

The system SHALL record that the derived scope has been offered, in browser storage
separate from `hire.jobFilters`, and SHALL NOT apply the derived scope again on any
later visit from that browser — whether or not the visitor kept it.

The separate marker is required rather than convenient: clearing the filters removes
`hire.jobFilters` entirely, so a derived scope keyed on "storage is empty" would
re-apply on the next visit and silently undo the visitor's clear, every time.

#### Scenario: The visitor clears the guessed scope and returns

- **WHEN** a visitor is scoped to `latam` by the guess, clears the scope, and later opens a bare `/jobs` again
- **THEN** the unfiltered list is shown and the guess is not re-applied

#### Scenario: The visitor keeps the guessed scope and returns

- **WHEN** a visitor keeps the guessed scope, so it is persisted as an ordinary explicit filter once they next edit the filters, and later returns
- **THEN** the ordinary restore path supplies the scope, and the guess itself is not applied a second time

#### Scenario: The visitor wipes their browser storage

- **WHEN** the visitor clears their browser data, removing the marker along with `hire.jobFilters`
- **THEN** the browser is indistinguishable from a new one and the guess is offered again — wiping storage is the only thing that re-arms it

#### Scenario: Storage is unavailable

- **WHEN** browser storage throws or is disabled, so the marker can be neither read nor written
- **THEN** the derived scope SHALL NOT be applied at all, and the page works exactly as it does today

### Requirement: Scope is limited to the standalone jobs feed

The system SHALL apply the derived scope only to the standalone `/jobs` list. The
company list, the company-embedded jobs list, and every other surface carrying a
filter store SHALL be untouched.

A company's geography answers "where is this employer", which is a different
question from "where may I work from"; defaulting one from the other would filter a
list by a fact the visitor never asked about.

#### Scenario: The company list is unaffected

- **WHEN** a visitor from Brazil opens `/companies`
- **THEN** no region is applied and the full company list is shown

#### Scenario: A company's postings are unaffected

- **WHEN** a visitor from Brazil opens a company page carrying an embedded jobs list
- **THEN** that list is not scoped by the derived region
