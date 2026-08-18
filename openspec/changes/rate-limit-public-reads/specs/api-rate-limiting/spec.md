## ADDED Requirements

### Requirement: Rate-limit budget headers on every limited response

Every response from a rate-limited route SHALL carry `X-RateLimit-Limit`,
`X-RateLimit-Remaining` and `X-RateLimit-Reset`, on success as well as on
rejection.

The headers exist so a ceiling can be respected rather than merely discovered.
A limit disclosed only in the 429 that enforces it tells a client what it did
wrong after the request has already failed; a client that can read its remaining
budget can slow down before that point. `X-RateLimit-Reset` SHALL be whole
seconds until the caller's budget is full again, and `X-RateLimit-Remaining` the
count of further requests permitted in the current window.

The backend already computes all three values on every check. They SHALL be
carried through the `Throttler` contract rather than recomputed or estimated in
the middleware, so a caller's remaining budget can never disagree with the
decision that produced it.

A request allowed because the backend failed (see *Fail-open on backend error*)
SHALL carry no rate-limit headers. No check happened, so there is no budget to
report, and reporting a fabricated one would be worse than silence.

#### Scenario: Headers accompany an allowed request

- **WHEN** a caller makes a request to a rate-limited route while within its budget
- **THEN** the response is the route's normal success response and additionally
  carries `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `X-RateLimit-Reset`

#### Scenario: Remaining decreases across successive requests

- **WHEN** a caller makes two successive requests to the same rate-limited route
  inside one window
- **THEN** the `X-RateLimit-Remaining` on the second response is lower than on the
  first

#### Scenario: Headers accompany a rejection

- **WHEN** a caller exceeds a route's limit
- **THEN** the `429` response carries `Retry-After` as before, and additionally
  `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `X-RateLimit-Reset`

#### Scenario: A fail-open response reports no budget

- **WHEN** the rate-limit backend errors or times out and the request is allowed
  through
- **THEN** the response carries none of the `X-RateLimit-*` headers

### Requirement: Public read endpoints are rate-limited by cost class

The public, unauthenticated read endpoints SHALL enforce a request budget, split
into two classes by the cost of serving them.

**Every** public job and company read SHALL share one budget — the job list, job
search, job facets, a single job and its copies and apply form, similar jobs,
company search and its vocabulary, a single company, and city lookup. The set is
exhaustive by intent rather than by enumeration: leaving one read out would void
the limit rather than narrow it, since the job list returns the same catalogue as
job search, so an unbounded sibling is simply the door a throttled caller walks
through instead. The
agent-oriented job search SHALL have its own, smaller budget, because it
rehydrates every result's full description from the database and so costs
several times more per request in both bytes and latency than any other read. A
single shared budget would have to be sized for the expensive endpoint, and
would then throttle the cheap ones far harder than their cost justifies.

Each budget SHALL be namespaced separately, so exhausting one leaves the other
untouched, and SHALL identify an authenticated caller by user rather than by
address — not to grant a larger allowance, which this change does not do, but so
that callers sharing an egress address do not share an allowance.

#### Scenario: Exhausting the agent search budget leaves ordinary search available

- **WHEN** a caller exhausts the budget for the agent job-search endpoint
- **THEN** their next request to the ordinary job-search endpoint is still served

#### Scenario: A caller over the public read budget is refused

- **WHEN** an external caller exceeds a public read budget within its window
- **THEN** the response is `429 Too Many Requests` with `Retry-After` and the
  rate-limit headers

#### Scenario: Two callers do not share an allowance

- **WHEN** two distinct callers each make requests to the same public read endpoint
- **THEN** neither caller's requests count against the other's budget

### Requirement: Trusted internal callers are not rate-limited

A request arriving from a loopback or private-network peer SHALL bypass
rate-limit enforcement entirely, and SHALL carry no rate-limit headers.

The server-rendered front end reaches the API directly over loopback rather than
through the reverse proxy, and forwards no client-address header, so every
server-rendered page presents the same peer address. Counting those requests
would place the whole site in one caller's budget and throttle it as a single
abusive client.

The trusted set SHALL be exactly the proxy-trust list the server already uses to
decide whether an address may assert a client address on another's behalf.
Defining it twice invites the two to drift, and a peer trusted to speak for
other callers is by construction not one this limit defends against.

#### Scenario: A loopback caller is never refused

- **WHEN** a request reaches a rate-limited route from a loopback address, in
  volume far exceeding the route's configured limit
- **THEN** every request is served, and none carries rate-limit headers

#### Scenario: An external caller is still limited

- **WHEN** a request reaches the same route from a public address that exceeds the
  configured limit
- **THEN** it is refused with `429`

## MODIFIED Requirements

### Requirement: Existing per-route limits preserved

Migrating a route onto the shared backend SHALL NOT change its configured limit,
window, or rejection status code, and SHALL NOT remove any header the route
already sent.

Adding the budget headers is additive: a route that answered `429` with
`Retry-After` still answers `429` with `Retry-After`, and gains
`X-RateLimit-*` alongside. No caller relying on the previous behaviour observes
a regression; a caller ignoring the new headers is unaffected.

#### Scenario: Request over the limit is rejected the same way

- **WHEN** a caller exceeds a route's configured limit within its configured window
- **THEN** the response is `429 Too Many Requests` with a `Retry-After` header, as it
  was before migrating that route onto the shared backend

#### Scenario: An existing limiter's configured budget is unchanged

- **WHEN** a route that was rate-limited before this change is exercised up to and
  past its limit
- **THEN** it admits exactly the same number of requests in the same window as it
  did before
