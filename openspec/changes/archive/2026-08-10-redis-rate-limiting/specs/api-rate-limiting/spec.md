## ADDED Requirements

### Requirement: Shared rate-limit backend
Every rate-limited HTTP route SHALL enforce its limit through one shared `Throttler`
backend rather than a per-route, per-process counter.

#### Scenario: Two different routes do not share a counter
- **WHEN** the same authenticated user exhausts the limit on one rate-limited route
- **THEN** their request budget on every other rate-limited route is unaffected

#### Scenario: Limit survives a process restart
- **WHEN** the API server process restarts after a user has partially consumed a route's
  limit within the current window
- **THEN** the user's remaining budget for that window reflects requests already made
  before the restart, not a reset counter

### Requirement: Route-namespaced rate-limit keys
Every rate-limit key SHALL be namespaced by the route it protects, so that no two distinct
routes can ever produce the same key for the same caller.

#### Scenario: Same user, two different routes, no bare identifier key
- **WHEN** a request is rate-limited using the caller's user ID or IP address as part of the
  key
- **THEN** the generated key includes a route-specific prefix and cannot collide with the
  key generated for a different route for the same user ID or IP address

### Requirement: Fail-open on backend error
If the rate-limit backend is unreachable or errors, the affected request SHALL be allowed
through rather than rejected, and the failure SHALL be logged.

#### Scenario: Backend unreachable
- **WHEN** the rate-limit backend cannot be reached (connection error or timeout) while
  checking a request against its limit
- **THEN** the request proceeds as if it were within the limit, and a warning is logged

#### Scenario: Backend call is bounded in time
- **WHEN** the rate-limit backend does not respond promptly
- **THEN** the check gives up after a short bounded timeout and treats the request as
  allowed, rather than blocking the request indefinitely

### Requirement: Existing per-route limits preserved
Migrating a route onto the shared backend SHALL NOT change its configured limit, window, or
client-visible rejection behavior.

#### Scenario: Request over the limit is rejected the same way
- **WHEN** a caller exceeds a route's configured limit within its configured window
- **THEN** the response is `429 Too Many Requests` with a `Retry-After` header, as it was
  before migrating that route onto the shared backend

### Requirement: Conditional rate limiting for content-dependent routes
A route SHALL be permitted to skip rate-limit enforcement based on properties of the
individual request (for example, an empty payload that triggers no downstream work),
independent of the shared backend's key/limit/window contract.

#### Scenario: Request with no actionable payload is not counted
- **WHEN** a request to a conditionally-limited route carries no payload that would trigger
  the work the limit exists to bound
- **THEN** the request is not counted against the caller's limit for that route
