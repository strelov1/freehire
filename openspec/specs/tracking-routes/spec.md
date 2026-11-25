# tracking-routes Specification

## Purpose
The /me/tracking route and its redirects from the old URL. Split out of user-job-tracking.

## Requirements

### Requirement: Tracking routes moved to /me/tracking

The per-user tracking endpoints SHALL be served under `/api/v1/me/tracking` (`""`, `/viewed`, `/pipeline`, `/swipe`, `/analyses`), replacing the previous `/api/v1/me/jobs*` paths, which MUST no longer be registered. This is a breaking API change: clients (the freehire-cli) MUST migrate to `/me/tracking`.

#### Scenario: Canonical tracking path

- **WHEN** a caller requests `GET /api/v1/me/tracking`
- **THEN** it returns the caller's tracked jobs (the listing previously served at `/api/v1/me/jobs`)

#### Scenario: Old path is gone

- **WHEN** a client requests `GET /api/v1/me/jobs`
- **THEN** the system returns `404` — the path is no longer registered

