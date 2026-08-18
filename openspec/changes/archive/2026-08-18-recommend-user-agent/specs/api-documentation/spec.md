## ADDED Requirements

### Requirement: Published client identification convention

The published API documentation SHALL state a requested `User-Agent` format for
programmatic callers, and SHALL state that it is requested rather than required.

The recommended shape is `owner/project/version (+contact-url)` — the version
and contact URL optional, the owner and project name not. The documentation
SHALL say what identifying buys the caller: contact before a limit changes,
instead of a `429` as first notice.

The API SHALL NOT validate, require, or behave differently on the header. No
request is refused, delayed, budgeted differently, or logged as an error for
omitting it or for sending anything at all. Enforcement is deliberately
deferred: the convention is published to callers who predate it, and refusing
them for not following an instruction that did not exist when they integrated
would break working clients to enforce a courtesy.

This is recorded as a decision rather than left implicit, so that a later
reader finds a considered deferral instead of an unfinished feature. Should
identification ever gate anything, it SHALL be through a credential the server
issues, not a self-declared string a caller can set to any value.

#### Scenario: A caller sending no user agent is served normally

- **WHEN** a request arrives at any public endpoint with no `User-Agent` header,
  or with a generic HTTP-library default
- **THEN** it is served exactly as an identified caller's request would be, with
  the same rate-limit budget and the same response

#### Scenario: The convention is discoverable where integrators look

- **WHEN** an integrator reads the published schema, the llms.txt summary, or
  robots.txt
- **THEN** each states the requested format and that it is not enforced
