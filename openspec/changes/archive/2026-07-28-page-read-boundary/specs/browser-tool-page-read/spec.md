## ADDED Requirements

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
