## ADDED Requirements

### Requirement: Browser-compatible Computrabajo posting hrefs

The Computrabajo adapter MUST remove ASCII tab, carriage return and line feed from
a posting href before validating its path and constructing the detail request URL.

#### Scenario: Embedded control whitespace in a posting slug

- **WHEN** a valid listing card contains tab or newline characters inside its href
- **THEN** its URL is the same URL a browser would request without those characters
- **AND** its posting identity and title remain unchanged
- **AND** the resulting URL can construct an HTTP GET request
