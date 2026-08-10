# browser-tool-page-read Specification

## Purpose

The `read_page` primitive this extension answers over the browser-tool wire, which
is how freehire's assistant sees the page the candidate is on.

The relay and the tool that calls it live in hire; this capability is the
extension's end of that wire.

## Requirements

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

### Requirement: Only ordinary web pages are read

`read_page` SHALL serve only a tab whose url is `http` or `https`. Any other tab —
a browser settings page, an extension page, a local file — SHALL be refused with an
error naming the rule, so the caller can tell the user why rather than reporting a
failure it cannot explain.

The refusal SHALL be decided from the url, before the page is read, since the point
is to not read it.

#### Scenario: A browser page is refused

- **WHEN** `read_page` is called while the active tab is a `chrome://` page
- **THEN** the call is answered with an error saying only ordinary web pages can be read, and the page is not read

#### Scenario: A local file is refused

- **WHEN** `read_page` is called while the active tab is a `file://` url
- **THEN** the call is answered with the same error

#### Scenario: An ordinary page is served

- **WHEN** `read_page` is called on an `https` page
- **THEN** it is read and answered as before
