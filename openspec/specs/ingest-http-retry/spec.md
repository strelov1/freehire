# ingest-http-retry Specification

## Purpose

Defines which response shapes the shared ingest HTTP client treats as transient (worth
retrying against a bounded budget) versus terminal (returned to the caller immediately), so a
source adapter's crawl fails only on genuine, persistent problems rather than on a one-off
network or edge hiccup.

## Requirements

### Requirement: Transient failures are retried against a bounded budget

The shared ingest HTTP client SHALL retry a request up to a fixed number of additional
attempts, with a backoff delay between attempts, when the failure is one of: a network-level
error (the request could not be sent or the connection failed), an HTTP `429` response (delay
honors a `Retry-After` hint when present), or an HTTP `5xx` response. Once the retry budget is
exhausted, the client SHALL return the last such failure to the caller.

#### Scenario: A transient server error recovers on retry

- **WHEN** a request receives a `5xx` response and a subsequent attempt (within the retry
  budget) receives a `2xx` response with a body that decodes successfully
- **THEN** the client returns the decoded result with no error

#### Scenario: Persistent server errors exhaust the retry budget

- **WHEN** every attempt within the retry budget receives a `5xx` response
- **THEN** the client returns an error identifying the last status received

### Requirement: An empty-bodied success is treated as transient, not terminal

The client SHALL treat a `2xx` response whose body decodes as exactly empty (the decoder's
own "no content" signal, distinct from a non-empty body that fails to decode) as a transient
failure, retried against the same bounded budget as a `5xx` response. This is scoped narrowly
to a genuinely empty body: an edge that answers success with nothing to read is behaviorally
indistinguishable from a dropped connection, and the client already retries that shape.

#### Scenario: An empty-bodied response recovers on retry

- **WHEN** a request receives a `2xx` response with an empty body, and a subsequent attempt
  (within the retry budget) receives a `2xx` response with a body that decodes successfully
- **THEN** the client returns the decoded result with no error

#### Scenario: A persistently empty body exhausts the retry budget

- **WHEN** every attempt within the retry budget receives a `2xx` response with an empty body
- **THEN** the client returns an error, exactly as a persistently `5xx` board would

#### Scenario: A non-empty but malformed body is never retried

- **WHEN** a request receives a `2xx` response whose body is present but fails to decode (e.g.
  invalid markup partway through the document, not an empty body)
- **THEN** the client returns the decode error immediately, without spending any of the retry
  budget

### Requirement: Non-transient failures return immediately

The client SHALL return certain failures to the caller on the first occurrence, without
retrying on the same egress: an HTTP `4xx` response other than `429` (a request that will not
succeed by being repeated unchanged against the same address), and a bot-mitigation challenge
response (retrying only deepens a per-address penalty rather than clearing it). An HTTP `403`
MAY be retried exactly once, but only through a distinct fallback egress when the client is
configured with one — never by repeating the request unchanged against the same address.

#### Scenario: A client error is not retried

- **WHEN** a request receives an HTTP `4xx` response other than `429` or `403`
- **THEN** the client returns immediately without attempting a retry

#### Scenario: A 403 without a fallback egress is not retried

- **WHEN** a request receives an HTTP `403` response and the client has no fallback egress
  configured
- **THEN** the client returns immediately without attempting a retry

#### Scenario: A bot-mitigation challenge is not retried

- **WHEN** a request receives a response identified as an automated-traffic challenge rather
  than the requested content
- **THEN** the client returns immediately without attempting a retry
