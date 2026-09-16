## Why

Every production stack trace in Sentry arrives minified. The frontend build uploads
source maps with `SENTRY_AUTH_TOKEN`, and the token on the host is rejected —
`sentry-cli` answers `Invalid token (http status: 401)` on every release, most recently
2026-09-16T04:30:20Z. The upload fails, the build carries on, and the only trace is a
stack dump in `journalctl -u freehire-autodeploy`.

Nobody found this by being told. It surfaced while reading an incident, where the frames
naming our own code were `fn=t` and `fn=f` and the failing line had to be recovered by
counting lines in the source by hand. That is the cost being paid on every investigation,
and it is paid silently.

`web/vite.config.ts` documents one failure mode and not the one in production:

> Source-map upload is inert without `SENTRY_AUTH_TOKEN` — the build still succeeds,
> only readable minified stack traces in Sentry are skipped.

That is true of an **absent** token, and it is also true of an **invalid** one — which is
the problem. `@sentry/sveltekit` catches the failed upload itself, prints a warning, and
lets the build exit 0, so a deliberate opt-out and a broken credential are
indistinguishable from the outside, and the text that would have told a reader otherwise
is describing the case we are not in. This is the same hazard `docs/` already records
elsewhere: prose about code is tested by nothing.

## What Changes

- **An invalid credential fails the release.** When `SENTRY_AUTH_TOKEN` is set and Sentry
  rejects it, the release MUST stop rather than warn and continue. A token that was typed
  is a claim that source maps are wanted; a release that silently drops them is not the
  thing that was asked for. The check cannot live inside the build — the bundler's exit
  status structurally cannot carry the answer — so it sits in the release path, before the
  color switch, the same posture `migrations FAILED — not touching green or the live color`
  takes.
- **An absent credential stays a clean, stated opt-out.** No token MUST remain inert and
  MUST say so once, in one line, so "off on purpose" and "broken" never read alike.
- **The claim becomes checkable.** A release that uploaded source maps can be told from
  one that did not, without reading a build log.
- Replace the dead credential on the host, and record where it lives.

## Capabilities

### New Capabilities

_None._ This tightens an existing capability rather than introducing one.

### Modified Capabilities

- `error-tracking`: adds a requirement covering source-map upload — that a rejected
  credential fails the build, an absent one is a stated no-op, and the outcome is
  observable after the fact. The existing spec covers what is reported and what is not;
  it says nothing about whether a report can be read once it arrives.

## Impact

- `web/vite.config.ts` — the `sentrySvelteKit()` source-maps options, and the comment
  that currently describes only the absent-token case.
- `deploy/` — the release path's verification step, and the host's environment file that
  holds the credential. Per `deploy/AGENTS.md` nothing in `deploy/` deploys itself, so a
  host copy is part of the work, not a follow-up.
- No runtime behaviour on the site changes: this is build- and release-time only. The
  error-reporting paths themselves are untouched.

### Out of scope, but found on the way

The frontend's transient-noise filter (`web/src/lib/sentryNoise.ts`, shipped in #2892)
extends the existing spec's "the inbox reflects genuine faults rather than routine
traffic" principle to the browser and SSR, and is currently recorded in no spec. It is
left out of this change deliberately rather than folded in, so the scope stays the one
that was asked for — but it is a real gap in `error-tracking` and wants its own delta.
