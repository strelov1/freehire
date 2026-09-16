## Why

The `error-tracking` spec says the frontend MUST capture unhandled errors, without
qualification. Production no longer does, and has not since #2892: `web/src/lib/sentryNoise.ts`
drops transport failures, aborts, and stale-chunk imports through a `beforeSend` on both
hooks, and `internal/api/handler`'s `classify()` stopped reporting an autopilot run refused
for want of a fit analysis.

The code is right. The spec is what is wrong, and it is wrong in the direction that gets
working code deleted: a reader comparing the two concludes the filter is a bug against the
requirement, removes it, and the free tier's 5 000 errors a month go back to being spent on
the same thing they were spent on before — a month's allowance exhausted on the 14th, with
a production outage on the 15th thrown away unseen.

The `restore-sentry-sourcemaps` proposal named this gap and deliberately left it out to keep
that change's scope honest. This is the change that pays it.

## What Changes

- The frontend capture requirement gains the distinction the rest of the capability already
  makes: report a DEFECT, not a CONDITION. Transport, cancellation and a browser holding
  markup a deploy replaced are conditions — observable in nginx, the uptime check and the
  release log, and not fixed by reading a thousand copies of them.
- The backend requirement's list of what is NOT reported stops being an enumeration that
  drifts. It already omitted `fitanalysis.ErrNoAnalysis`, which predates this work, as well
  as `search.ErrBadQuery`, the `context.Canceled` 499, and now the autopilot refusal. The
  list becomes the rule those entries are instances of, with `classify()` named as the one
  place that decides.
- **No code changes.** Nothing in this change alters behaviour; it makes the specification
  describe what ships.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `error-tracking`: two requirements. "Frontend client and server error capture" currently
  requires unconditional capture; "Backend panic and unexpected-error capture" carries a
  closed list of exceptions that is already incomplete.

## Impact

- `openspec/specs/error-tracking/spec.md` only, via this change's delta.
- No production behaviour, no runtime code, no tests. The behaviour being described is
  already live and already tested — `web/src/lib/sentryNoise.test.ts` pins what is dropped
  and, more importantly, what must still get through, and `internal/api/handler/errors_test.go`
  pins the backend side.
