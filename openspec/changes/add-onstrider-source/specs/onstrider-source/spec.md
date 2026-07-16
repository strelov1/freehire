## ADDED Requirements

### Requirement: Onstrider enumerates vacancies from the sitemap

The system SHALL provide an `onstrider` source adapter that enumerates vacancy URLs from
`https://www.onstrider.com/sitemap.xml`. The adapter SHALL keep only canonical vacancy URLs
of the form `https://www.onstrider.com/jobs/<slug>-<8hex>` and SHALL drop every other entry —
blog, marketing, localized (`/pt/`, `/en/`), and `/preview-slug-…/` duplicate URLs — so each
vacancy is fetched at most once.

The adapter is **boardless** (onstrider.com is one site with no per-tenant board). The
configured board file entry carries no `board` value.

#### Scenario: Sitemap yields only canonical vacancy URLs

- **WHEN** the adapter reads a sitemap containing canonical `/jobs/<slug>-<8hex>` URLs
  alongside blog, marketing, localized, and `/preview-slug-…/` URLs
- **THEN** it fetches only the canonical `/jobs/<slug>-<8hex>` URLs and ignores the rest

### Requirement: Onstrider maps an open vacancy's JobPosting ld+json to a Job

The system SHALL fetch each canonical vacancy page and map its embedded schema.org
`JobPosting` ld+json to a normalized `Job`. The `Job` SHALL carry the `identifier.value`
(a UUID) as its `ExternalID`, the vacancy page URL as its canonical URL, the posting title,
the sanitized HTML `description`, the company name `Strider` (the site's `hiringOrganization`
is always Strider, the real employer being hidden), the countries from
`applicantLocationRequirements[].name`, and `Remote: true` with work mode `remote` when
`jobLocationType` is `TELECOMMUTE`. Posted-at SHALL be taken from `datePosted` when present.

#### Scenario: An open vacancy page maps to a job

- **WHEN** the adapter fetches a canonical vacancy page carrying a `JobPosting` ld+json block
- **THEN** it yields one `Job` with `ExternalID` set to the posting's `identifier.value`, the
  page URL as the canonical URL, the title, the sanitized description, company `Strider`, the
  `applicantLocationRequirements` countries as its location, and remote work mode when
  `jobLocationType` is `TELECOMMUTE`

### Requirement: Onstrider treats a missing JobPosting block as a closed vacancy

The adapter SHALL treat the presence of the `JobPosting` ld+json block as the open/closed
signal, dropping any vacancy URL whose page carries no `JobPosting` block. The sitemap retains
closed vacancy URLs indefinitely and they still return HTTP 200, but a closed vacancy drops its
`JobPosting` markup. The adapter SHALL also drop any posting lacking a usable `identifier.value`
(which would collide on the dedup key). A single dropped or failed vacancy SHALL NOT abort the
crawl; vacancies that stop being emitted are soft-closed by the standard ingest sweep.

#### Scenario: A closed vacancy is dropped

- **WHEN** the adapter fetches a canonical vacancy URL whose page has no `JobPosting` ld+json
  block (the vacancy has been closed)
- **THEN** the adapter yields no `Job` for that URL and continues with the remaining URLs

#### Scenario: A vacancy without a usable identifier is dropped

- **WHEN** a fetched vacancy carries a `JobPosting` block whose `identifier.value` is empty
- **THEN** the adapter drops that posting and continues mapping the rest

### Requirement: Onstrider crawl egresses through the proxy allowlist

The system SHALL register `onstrider` in `proxiedProviders` so that, when `SOURCES_PROXY_URL`
is set, the onstrider crawl egresses through that proxy — the site sits behind Cloudflare and
the edge may block the prod datacenter IP. When `SOURCES_PROXY_URL` is unset the provider
crawls over the direct client unchanged.

#### Scenario: Proxy egress is applied when configured

- **WHEN** `SOURCES_PROXY_URL` is set and `sources.ApplyProxyEgress` runs over the registry
- **THEN** the `onstrider` adapter's transport is routed through the configured proxy while
  non-allowlisted providers stay on the direct client
