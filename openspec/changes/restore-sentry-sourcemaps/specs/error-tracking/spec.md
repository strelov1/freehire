## ADDED Requirements

### Requirement: Frontend source maps are uploaded, or the release says they were not

A reported error is only useful if the frames can be read. The frontend build ships
minified JavaScript, so the browser and SSR stack traces that reach Sentry name our code
only after the matching source maps have been uploaded for that release.

The release path SHALL establish which of the two states it is in, and SHALL NOT allow
them to be confused:

- When an upload credential is configured and Sentry REJECTS it, the release MUST fail. A
  configured credential is a statement that readable traces are wanted; shipping without
  them is not the release that was asked for.
- When the credential cannot be checked at all — Sentry unreachable, or answering with a
  fault of its own — the release MUST continue and MUST say that it could not verify. A
  rejection is proof of a misconfiguration; an unreachable checker is proof of nothing,
  and a release path that stops on it makes our ability to ship depend on somebody else's
  uptime.
- When no upload credential is configured, source-map upload MUST remain a no-op, the
  release MUST succeed, and it MUST state once that traces for this release will be
  minified — so a deliberate opt-out is never mistaken for a broken credential.

Verification MUST NOT be inferred from the bundler's own exit status. The SvelteKit
Sentry plugin catches an upload failure, prints a warning, and lets the build succeed, so
a build that exits 0 is not evidence that anything was uploaded.

#### Scenario: A rejected credential fails the release

- **WHEN** the web build runs with an upload credential configured and Sentry rejects it as invalid
- **THEN** the release MUST fail, naming source-map upload as the reason
- **AND** the live deployment MUST be left untouched

#### Scenario: An accepted-but-under-scoped credential fails the release

- **WHEN** the configured credential is valid but lacks the permission the source-map upload needs
- **THEN** the release MUST fail, naming the missing permission
- **AND** the live deployment MUST be left untouched

#### Scenario: An unreachable Sentry does not stop the release

- **WHEN** the credential cannot be checked because Sentry is unreachable or answers with a fault of its own
- **THEN** the release MUST continue and MUST state that the credential could not be verified

#### Scenario: A successful upload lets the release continue

- **WHEN** the web build runs with a valid upload credential and the release's source maps reach Sentry
- **THEN** the release MUST continue

#### Scenario: No credential is a stated no-op

- **WHEN** the web build runs with no upload credential configured
- **THEN** no upload MUST be attempted, the release MUST continue, and it MUST report that this release's traces will be minified

#### Scenario: A build that exits 0 is not taken as proof

- **WHEN** the bundler completes successfully but no source maps were uploaded for the release
- **THEN** the release MUST still fail if a credential was configured, rather than treating the bundler's exit status as evidence of upload
