## ADDED Requirements

### Requirement: The release establishes whether source maps can upload, and says so

A reported error is only useful if the frames can be read. The frontend build ships
minified JavaScript, so the browser and SSR stack traces that reach Sentry name our code
only after the matching source maps have been uploaded for that release.

The release path SHALL establish, independently of the bundler, whether this release's
source maps CAN upload, and SHALL report which state it is in:

- When an upload credential is configured and Sentry rejects it — as invalid, as lacking
  the permission the upload needs, or as naming an organisation or project that does not
  exist — the release MUST fail. A configured credential is a statement that readable
  traces are wanted; shipping without them is not the release that was asked for.
- When an upload credential is only partly configured, the release MUST fail and MUST name
  what is missing. A partial configuration uploads nothing, exactly like an opt-out, so it
  MUST NOT be reported as one.
- When the credential cannot be checked at all — Sentry unreachable, answering with a fault
  of its own, or the check itself unable to run — the release MUST continue and MUST say it
  could not verify. A rejection is proof of a misconfiguration; none of these is proof of
  anything, and a release path that stops on them makes our ability to ship depend on
  somebody else's uptime.
- When no upload credential is configured, source-map upload MUST remain a no-op, the
  release MUST succeed, and it MUST state that traces for this release will be minified —
  so a deliberate opt-out is never mistaken for a broken credential.

The credential MUST be checked against a Sentry operation requiring the same permission the
source-map upload requires, so that a credential the upload cannot use cannot pass the
check.

This requirement is about whether upload is POSSIBLE, not about whether it HAPPENED. A
credential Sentry accepts, whose upload nevertheless produces no artifact, is not covered:
establishing that would need a positive control, and at the time of writing no release in
the observable window had ever uploaded successfully. That gap is deliberate and named
rather than implied.

#### Scenario: A rejected credential fails the release

- **WHEN** the release runs with an upload credential configured and Sentry rejects it as invalid
- **THEN** the release MUST fail, naming source-map upload as the reason
- **AND** the live deployment MUST be left untouched

#### Scenario: An under-scoped credential fails the release

- **WHEN** the configured credential is valid but lacks the permission the source-map upload needs
- **THEN** the release MUST fail, naming the missing permission
- **AND** the live deployment MUST be left untouched

#### Scenario: A credential naming the wrong org or project fails the release

- **WHEN** the configured organisation or project does not exist on the Sentry the release is pointed at
- **THEN** the release MUST fail, naming the settings to check

#### Scenario: A half-configured credential fails the release

- **WHEN** some but not all of the upload settings are configured
- **THEN** the release MUST fail and MUST name the settings that are missing
- **AND** it MUST NOT report this as a deliberate opt-out

#### Scenario: An unreachable Sentry does not stop the release

- **WHEN** the credential cannot be checked because Sentry is unreachable or answers with a fault of its own
- **THEN** the release MUST continue and MUST state that the credential could not be verified

#### Scenario: A check that cannot run does not stop the release

- **WHEN** the check itself cannot run — it is absent from the checkout, or its runtime is unavailable
- **THEN** the release MUST continue and MUST state that the credential could not be verified
- **AND** it MUST NOT report this as a rejected credential

#### Scenario: An accepted credential lets the release continue

- **WHEN** the release runs with an upload credential Sentry accepts
- **THEN** the release MUST continue, naming the organisation and project it was accepted for

#### Scenario: No credential is a stated no-op

- **WHEN** the release runs with no upload credential configured
- **THEN** no upload MUST be attempted, the release MUST continue, and it MUST report that this release's traces will be minified

#### Scenario: The bundler's exit status is not taken as evidence

- **WHEN** the bundler completes successfully
- **THEN** that MUST NOT be treated as evidence that the credential works or that anything was uploaded, because the SvelteKit Sentry plugin catches an upload failure, warns, and lets the build exit 0
