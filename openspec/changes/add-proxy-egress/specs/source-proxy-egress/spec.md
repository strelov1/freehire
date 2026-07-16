## ADDED Requirements

### Requirement: Opt-in per-provider proxy egress

The ingest system SHALL route a provider's outbound HTTP requests through a configured egress proxy only when that provider is explicitly marked as proxied. All other providers SHALL continue to use the direct connection.

#### Scenario: Proxied provider routes through proxy

- **WHEN** a proxy URL is configured and an adapter marked as proxied (e.g. `eightfold`) makes an HTTP request during ingest
- **THEN** the request egresses through the configured proxy, not the direct datacenter IP

#### Scenario: Non-proxied provider stays direct

- **WHEN** a proxy URL is configured and an adapter NOT marked as proxied makes an HTTP request
- **THEN** the request egresses over the direct connection, bypassing the proxy

### Requirement: Proxy is disabled by default

The system SHALL treat proxy egress as absent when no proxy is configured, leaving all providers on the direct connection with behavior identical to a build without proxy support.

#### Scenario: No proxy configured

- **WHEN** the proxy configuration is empty and any adapter (including one marked as proxied) makes a request
- **THEN** the request egresses over the direct connection and ingest proceeds unchanged

#### Scenario: Invalid proxy configuration fails fast

- **WHEN** a proxy URL is configured but malformed or unusable
- **THEN** the worker surfaces a startup/construction error rather than silently falling back to the direct IP for a proxied provider

### Requirement: Proxy credentials are not leaked

The system SHALL keep proxy credentials out of logs, error messages, and persisted state.

#### Scenario: Proxy error omits credentials

- **WHEN** a proxied request fails and the error is logged or recorded in board health
- **THEN** the recorded message does not contain the proxy username or password
