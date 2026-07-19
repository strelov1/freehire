# waf-challenge-pacing Specification

## Purpose
TBD - created by archiving change clinch-detail-via-paced-challenge-layer. Update Purpose after archive.
## Requirements
### Requirement: WAF challenge responses are surfaced as a typed error

The sources HTTP transport SHALL treat an AWS-WAF *Challenge* response as a
retrievable-content failure, not as a successful body. A response is a challenge
when it carries the `x-amzn-waf-action: challenge` header (AWS returns it with
HTTP status 202). The transport SHALL NOT hand the challenge shell to the
response decoder, and SHALL return a typed `ChallengeError` that callers can
match with `errors.As`.

#### Scenario: Challenge response is not decoded as content

- **WHEN** a request receives a response with header `x-amzn-waf-action: challenge`
- **THEN** the transport returns a `ChallengeError` (matchable via `errors.As`)
- **AND** the response body is not passed to the caller's decode function

#### Scenario: A normal 2xx is still decoded

- **WHEN** a request receives HTTP 200 without the challenge header
- **THEN** the transport decodes the body as it does today, unchanged

#### Scenario: Challenge is not retried as a transient failure

- **WHEN** a request receives a challenge response
- **THEN** the transport returns the `ChallengeError` immediately without consuming a retry attempt

### Requirement: Optional per-source request pacing under a shared limiter

The transport SHALL provide an optional rate-limited HTML getter that any source
may opt into, holding that source's aggregate `GetHTML` request rate under a
configured interval independent of worker concurrency. All requests routed
through one paced getter — across boards and both listing and detail paths —
SHALL share a single token bucket, following the existing `rateLimitedHTMLGetter`
pattern.

#### Scenario: Paced getter gates requests through one shared limiter

- **WHEN** a source is wired to a paced getter with interval `I` and burst `B`
- **THEN** its `GetHTML` calls are admitted at the shared limiter's rate, regardless of how many workers issue them concurrently

#### Scenario: A cancelled context surfaces before the inner fetch

- **WHEN** the context is cancelled while a request waits on the limiter
- **THEN** the getter returns the context error and does not perform the inner fetch

