# Headless browser transport

Launching a headless Chrome, and fetching URLs through one. Transport, not a feature: this
package knows nothing about jobs, candidates or application forms, in the same sense
`platform/llm` and `platform/aigateway` know nothing about what they carry bytes for.

It is in `platform` because two blocks need it and neither may import the other —
`api/atsapply` (block 8) drives a browser to fill an application form, `ingest/sources`
(block 7) needs one to read a source behind a JavaScript challenge. `blocks.go` records the
same argument for `aigateway`.

**Not to be confused with `platform/browseruse`**, its neighbour: that is an HTTP client for
somebody else's *hosted* browser (browser-use.com). This one launches a process on our own
host.

## What it owns

- **`LaunchOptions` / `LaunchOptionsThroughProxy`** — the stealth launch flags, and the single
  home for them in the repository. A second copy would mean the next anti-bot fix lands in one
  caller and silently not the other.
- **`Session`** — a running browser, a small pool of tabs, and `Fetch(ctx, url) (status, body,
  error)`.

What it deliberately does NOT own is what a caller then does with a page. `atsapply` scans a
DOM and submits a form; `sources` reads a body. An abstraction over those two would be an
abstraction over nothing.

## The three things measured, not assumed

Each of these was found by running against a real bot wall (echojobs.io, behind Vercel's, on
2026-09-07 — freehire#2588), and each is a trap the obvious implementation falls into.

**A browser alone is not enough, and neither is a proxy.** From the production datacenter IP
both a plain GET and a headless browser get `403 x-vercel-mitigated: deny` — no challenge is
offered at all, so there is nothing for a browser to solve. Through the egress proxy a plain
GET gets the challenge and cannot pass it. Only the pair works. That is why the ingest side
refuses to launch a browser without a proxy configured.

**The user agent is the loudest tell, and no launch flag removes it.** A hand-rolled probe
cleared the wall in eight seconds while this package sat refused for twenty, with identical
flags, proxy and tab structure — the probe set a desktop user agent and the package did not,
so Chrome announced `HeadlessChrome`. `hideHeadlessUserAgent` reads the running browser's own
agent and rewrites that one word. It is read rather than written as a literal so it never
claims a Chrome version the binary is not; for a fingerprint, a confident lie is worse than
nothing.

**Do not retry a refused request to find out whether the wall lifted.** These walls answer with
a rate-limit-flavoured `429`, so a burst of refused fetches is what holds them up. Clearance is
detected by WATCHING: navigate once, and wait for the main document response to turn 2xx — the
challenge serves the document with a refusal and then reloads itself. No extra request, no
site-specific marker, and a ceiling rather than a delay.

## Why the fetch happens inside the page

`Session.Fetch` evaluates `fetch()` in the cleared page rather than navigating to the URL.
Same clearance, same cookies, same TLS identity, but only the bytes come back: 0.36s against
4.2s for a navigation, measured.

The cheaper-looking idea does not work and was measured too — the clearance cookie handed to
`net/http` still gets the challenge back, so clearance is bound to more than the cookie and
the request has to leave from the browser itself.

A non-2xx is returned as a **status, not an error**. Only some statuses may be read as "this
resource is gone"; collapsing them would take that decision away from the caller. `sources`
turns them into its own `*StatusError` so a browser-fetched 404 still means what it means.

## Credentials

Chrome takes its proxy as a command-line argument, and a command line is readable by every
process on the host. So the scheme and host go there and **the credentials never do** — they
are answered over the debugging protocol (`fetch.EventAuthRequired`). A unit test scans the
flag set for a leaked password, and the error text for a malformed proxy is redacted, because
an error about a bad proxy must not be the thing that prints it.

## Testing

`session_integration_test.go` is `//go:build integration` and **skips when no Chrome is
installed** — CI's backend job has none, and a red build there would mean nothing. It drives a
local server that serves its content only after a JS-set cookie: the smallest thing shaped
like the real obstacle.

The clearance test asserts the challenge count **stops growing**, not that it equals one.
Pinning the exact number would fail on a slow machine for a reason that is not a bug.
