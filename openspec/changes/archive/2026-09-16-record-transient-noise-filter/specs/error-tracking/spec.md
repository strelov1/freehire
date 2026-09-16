## MODIFIED Requirements

### Requirement: Frontend client and server error capture

When active, the SvelteKit frontend SHALL report unhandled errors from both the browser
(client) and SSR (server) to Sentry via the framework error hooks, tagged with the
configured environment.

It SHALL NOT report a failure that is a transient CONDITION rather than a DEFECT. A
condition is transport (a dropped or refused connection, however each browser words it),
timing (an upstream read that did not answer inside the deadline the caller set),
cancellation (a request the app abandoned, or a visitor who navigated away), or a module
import failing because the deploy replaced the chunk an already-open tab still names. A
defect is anything that would still be wrong if the network were perfect.

The distinction is not a nicety. Conditions arrive one event per visitor per page, in the
thousands, exactly when the site is already known to be struggling, and they carry no stack
anyone can act on — what they record is that the backend was slow, which the host's own
latency metrics say more precisely and without cost. Measured over 2026-08-29..09-16, they
were ~70% of everything the organisation's month accepted; the allowance ran out on the
14th and every fault after it, including a production outage on the 15th, was rejected at
the door.

Whatever the filter cannot read, it SHALL report: a thrown value of no recognisable shape
is not evidence of a condition, and silence is the failure this filter must not introduce.

#### Scenario: Client-side error is reported

- **WHEN** an unhandled error that is not a transient condition is thrown while the app runs in the browser and `PUBLIC_SENTRY_DSN` is set
- **THEN** the error MUST be captured to Sentry from the client

#### Scenario: SSR error is reported

- **WHEN** an unhandled error that is not a transient condition is thrown during server-side rendering and `PUBLIC_SENTRY_DSN` is set
- **THEN** the error MUST be captured to Sentry from the server

#### Scenario: A transient condition is not reported

- **WHEN** the failure is a transport error, an upstream read that exceeded its deadline, an abandoned or cancelled request, or a module import naming a chunk the current deploy has replaced
- **THEN** it MUST NOT be captured to Sentry, from either the client or SSR

#### Scenario: An unreadable failure is still reported

- **WHEN** the thrown value has no shape the filter can inspect
- **THEN** it MUST be captured, rather than dropped on the assumption that it is a condition

### Requirement: Backend panic and unexpected-error capture

When active, the HTTP server SHALL report unhandled panics and unexpected server errors
(HTTP 5xx) to Sentry, while continuing to serve the existing JSON error envelope to the
client.

A failure that is an expected STATE rather than a fault SHALL NOT be reported, so the
Sentry inbox reflects genuine faults rather than routine traffic. This is a rule, not a
list: the decision lives in one place (`classify()` in `internal/api/handler`), which
returns both the status to render and whether the failure is worth reporting, and every
reader of it — the HTTP error handler and the streaming endpoints alike — gets the same
answer. Instances include any `*fiber.Error` with a 4xx status, a missing row or
foreign-key violation mapped to 404, a client that disconnected mid-request, a search query
the engine rejected as malformed, and a request refused because a prerequisite the
candidate controls has not been met.

An earlier wording enumerated three of those and was already incomplete when written. An
enumeration in a specification drifts from the code that implements it; naming the rule and
the single place that applies it does not.

#### Scenario: Recovered panic is reported

- **WHEN** a handler panics and the recover middleware catches it
- **THEN** the panic MUST be captured to Sentry with a stack trace
- **AND** the client MUST still receive the standard 500 JSON error response

#### Scenario: Unexpected 500 is reported

- **WHEN** a handler returns an error that the central error handler maps to HTTP 500
- **THEN** the error MUST be captured to Sentry

#### Scenario: Routine 4xx is not reported

- **WHEN** a handler returns a failure `classify()` resolves to a non-5xx status — a 4xx `*fiber.Error`, a missing row, a foreign-key violation, a client disconnect, a malformed search query, or an unmet prerequisite the caller can act on
- **THEN** the error MUST NOT be captured to Sentry
- **AND** the client MUST receive the existing mapped status and JSON envelope

#### Scenario: A streamed refusal is judged the same way

- **WHEN** an endpoint that has already begun streaming refuses the run and reports the failure
- **THEN** it MUST consult the same decision the HTTP error handler consults, so a refusal already explained to the caller on the stream is not also filed as a fault
