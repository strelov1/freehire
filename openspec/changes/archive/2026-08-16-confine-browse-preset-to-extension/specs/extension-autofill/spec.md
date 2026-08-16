## MODIFIED Requirements

### Requirement: Both autofill entry points share one assembly

The system SHALL assemble the contact block once and serve it to both the extension's
deterministic read (`GET /api/v1/me/autofill-profile`) and the agent-driven fill
(`POST /api/v1/me/autofill/run`), so the values a person sees in a form and the values the
agent grounds its plan in cannot diverge. The read endpoint remains authenticated by
either the website's cookie or a full-scope API key, same as any other `mw.key` route.
The agent-driven fill does not: see "The agent-driven fill is confined to the
extension's own connection" below for what authenticates it.

#### Scenario: The agent fills from the same block the endpoint serves

- **WHEN** the agent-driven autofill runs for a user
- **THEN** it grounds its plan in the same contact block the read endpoint would serve that
  user

## ADDED Requirements

### Requirement: The agent-driven fill is confined to the extension's own connection

`POST /api/v1/me/autofill/run` SHALL refuse a request that authenticated by the
website's session cookie or by an API key, even though both otherwise pass this
route's ordinary auth gate. It attaches to the caller's browser-tool channel
(`internal/browsertools.Hub`, keyed by user id, not session id) and WRITES into
whatever form the browser currently attached to that channel is showing — unlike a
read, there is no safe degraded behavior, so a request that did not authenticate with
the extension's own Bearer session JWT is refused outright rather than run against a
browser the caller on this surface never opened.

#### Scenario: A cookie-authenticated request is refused

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by the website's session cookie
- **THEN** the request is refused and no browser-tool call is made

#### Scenario: An API-key-authenticated request is refused

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by a full-scope API key
- **THEN** the request is refused and no browser-tool call is made

#### Scenario: The extension's own Bearer session JWT is admitted

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by a Bearer session JWT
- **THEN** the request proceeds to read the caller's browser-tool channel
