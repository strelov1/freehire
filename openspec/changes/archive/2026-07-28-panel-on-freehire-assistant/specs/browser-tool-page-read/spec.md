## ADDED Requirements

### Requirement: The extension answers a request to read the current page

The extension SHALL serve a `read_page` tool over the browser-tool wire, answering
with what the active tab is showing: its url, title, headline and visible text —
the same reading the panel's match card is built from.

It SHALL be answered from the live page at the moment of the call, not from
anything cached, because the caller asks precisely when it needs to know what the
user is looking at now.

#### Scenario: The page is read on request

- **WHEN** a `read_page` call arrives over the wire
- **THEN** the extension reads the active tab and answers with that page's url, title, headline and text

#### Scenario: The page cannot be read

- **WHEN** the active tab cannot be reached — no tab, or a page the extension may not touch
- **THEN** the call is answered with an error rather than left unanswered, so the caller is never left waiting

### Requirement: Every tool call is answered

The extension SHALL answer every well-formed call it receives, including one naming
a tool it does not implement. A caller is blocked on the id it sent; silence there
is indistinguishable from a hang.

#### Scenario: An unknown tool is named

- **WHEN** a call names a tool the extension does not implement
- **THEN** it is answered with an error carrying the same id
