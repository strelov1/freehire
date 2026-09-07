## ADDED Requirements

### Requirement: A source behind a JavaScript challenge is crawled through a browser

A source whose pages are gated behind a JavaScript challenge SHALL be crawled through a
headless browser session rather than a plain HTTP request, and the adapter that reads those
pages SHALL be unchanged by the switch — the browser supplies the same fetch operations the
adapter already declares.

#### Scenario: The adapter reads a challenged page

- **WHEN** a provider registered for the browser tier is crawled
- **THEN** its sitemap and its posting pages are fetched through the browser session
- **AND** the adapter parses them exactly as it parses plainly fetched ones

### Requirement: The browser tier requires an egress proxy

The browser tier SHALL be used only when an egress proxy is configured. Without one the
provider SHALL fall back to the plain client and fail as it would have anyway.

A browser and a proxy are each useless alone here: from an address the platform denies
outright no challenge is served, so there is nothing for a browser to solve; and a challenge
cannot be solved without executing JavaScript.

#### Scenario: No proxy configured

- **WHEN** a browser-tier provider is crawled and no egress proxy is configured
- **THEN** no browser is launched
- **AND** the crawl proceeds on the plain client

#### Scenario: Proxy configured

- **WHEN** a browser-tier provider is crawled and an egress proxy is configured
- **THEN** the browser session egresses through that proxy

### Requirement: A request rides a cleared page and reports its real status

A browser session SHALL obtain clearance once per tab by loading the site's origin, and
SHALL issue every subsequent request from inside that cleared page rather than by navigating
to it.

Each request SHALL report the response's own status code. A status SHALL NOT be collapsed
into a generic failure, because only some statuses may be read as "this posting is gone".

#### Scenario: Clearance is paid once, not per request

- **WHEN** a session fetches several URLs from one tab
- **THEN** the challenge is cleared on the first fetch only

#### Scenario: A missing page is distinguishable from a blocked one

- **WHEN** a fetch receives a `404`
- **THEN** the caller is told the status was `404`, not merely that the fetch failed

### Requirement: Proxy credentials never reach the process arguments

When the egress proxy requires authentication, the credentials SHALL be supplied to the
browser over its debugging protocol and SHALL NOT appear in the browser's command line.

#### Scenario: Launching with an authenticating proxy

- **WHEN** a session is launched with a proxy URL carrying a username and password
- **THEN** the browser's arguments carry the proxy host but neither the username nor the password

### Requirement: One home for the stealth launch flags

The flags that make a headless browser resemble an ordinary one SHALL be defined in exactly
one place, shared by every caller that launches a browser.

#### Scenario: A second caller launches a browser

- **WHEN** any package in the repository starts a headless browser
- **THEN** it takes its launch flags from the shared definition rather than restating them

### Requirement: A crawl that reads nothing of what it listed reports failure

A hydrating adapter that discovered candidate postings and successfully read none of them
SHALL report a board-level failure rather than an empty success.

An adapter drops a posting it cannot read before the pipeline ever sees it, so a total
outage and a genuinely empty crawl are otherwise the same result. This is what let one
source publish nothing for 19 days with every run reporting success.

#### Scenario: Every candidate fails to load

- **WHEN** a crawl lists candidate postings and none of them can be read
- **THEN** the board reports an error

#### Scenario: The source genuinely has nothing

- **WHEN** a crawl lists no candidate postings at all
- **THEN** the board reports success with nothing ingested
