## Why

The public API is unauthenticated, and now rate-limited (#2096). Both facts
make one question unanswerable: when a caller starts costing us something, who
do we talk to?

Measured on production, most substantial consumers already answer it
voluntarily — `ManyApplyAssist/6.0 (+https://manyeverything.xyz/apply-assist)`,
`BespokeJobDiscovery/...`, `freehire-search-skill/1.0 (+https://freehire.me)`.
The one that did not is instructive: a third-party integration shipped freehire
as its default job source while sending the shared default user agent of its
HTTP library, and wrote in its own PR that the traffic *"is not attributable to
freehire's own integration"*. That was their call to make, and they made it
explicitly — but nothing on our side ever asked.

Asking costs nothing and pays for itself the first time a client needs a higher
ceiling or a warning before a change. A limit without a way to reach the person
it constrains leaves us only one lever: refuse, and let them find out.

## What Changes

- **`openapi.yaml`, `llms.txt` and `robots.txt` publish a requested user-agent
  format** — `owner/project/version (+contact-url)`, the version and URL
  optional — and say plainly what it buys: a message before a limit changes,
  rather than a 429 as first contact.
- **It is a request, not a rule.** Nothing validates the header, nothing refuses
  a caller for omitting it, and no behaviour depends on it. Enforcing it today
  would break every client that predates the ask, which is the opposite of the
  goodwill the ask exists to build.
- The spec records that the enforcement is deliberately deferred, so a later
  reader finds a decision rather than an oversight.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `api-documentation`: the published schema gains a stated identification
  convention for API clients, explicitly unenforced.

## Impact

- `web/static/openapi.yaml`, `web/static/llms.txt`,
  `web/src/routes/robots.txt/+server.ts` — text only.
- No Go change, no middleware, no validation, no migration. A caller sending
  nothing today behaves identically tomorrow.

**Deliberately not done.** No enforcement, no soft warning header, no separate
budget for identified callers. A rate-limit tier keyed on a self-declared,
unverifiable string would be trivially forged, so it would have to arrive with
API keys rather than with a header convention — and that is a monetization
decision, not this one.
