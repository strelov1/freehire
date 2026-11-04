# Product monitoring

The API publishes product counters on the existing private Prometheus listener. Grafana
dashboards and alert provisioning live in `freehire-ops`, under
`provision/litellm-host/grafana/provisioning/`; the dashboard UID is `freehire-product`.

## Definitions

- `freehire_registrations_total`: newly persisted accounts. Password registration counts
  after insertion; OAuth counts after its link-or-create transaction commits. Existing
  account links, sign-ins, duplicate registrations and rolled-back creations do not count.
- `freehire_cv_uploads_total{outcome="success|error"}`: storage attempts after input
  validation. Success requires both object storage and the database pointer write. A
  subsequent metadata read failure does not undo that success. Guest parsing, invalid
  files, disabled storage and stateless extraction do not count. Replacements do count.
- `freehire_cv_extractions_total{outcome="success|error"}`: background structured-extraction
  attempts, including retries. Disabled extraction and missing upload stamps are errors.
  Success means extraction and the structured-write call returned without error; a
  superseded upload can be discarded by the write's stamp guard. Experience-bank import
  is a separate best-effort step and is not part of this outcome.

Counters expose zero before any event, contain no user identifiers or CV content, and
are independent of browser consent and PostHog. They reset on process restart: use
`sum(increase(...[window]))`, applying `increase` before summing blue and green.
These operational counters can miss events between the last scrape and process exit;
they are not an accounting ledger or a source of exact historical user counts.

## Alerts

Volume rules compare a rolling six-hour window to the same window one week earlier.
They fire after a drop greater than 70%, with at least ten events in the reference
window, sustained for thirty minutes. Both windows require six hours of telemetry
coverage; missing data is handled separately rather than read as zero activity.
The first volume comparison becomes available one week after rollout.

Storage and structured-extraction failures alert above 20% over thirty minutes, with
at least five attempts, sustained for ten minutes. Thresholds are initial operational
defaults: tune after observing traffic. A separate telemetry alert detects loss of any
product metric family while at least one API scrape target is reachable.

The dashboard shows registrations, stored CVs, storage success rate, extraction outcomes,
and current versus previous-week volume. Upload counts divided by registration counts
are deliberately not presented as activation conversion: uploads include existing users
and replacements. True registration-to-first-CV conversion needs a user cohort and a
durable first-upload event.

## Rollout

Deploy the API counter changes before enabling the telemetry alert. Provision
`dashboards/freehire-product.json` and `alerting/product.yaml` from `freehire-ops` on the
monitoring host. The existing `telegram-ops` notification policy routes these rules.
Confirm that the live API exposes every family, Prometheus scrapes them, and Grafana
loads the dashboard and all rules. A missing family is not evidence of zero usage.
