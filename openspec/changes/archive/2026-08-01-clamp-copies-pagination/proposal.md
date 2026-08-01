## Why

`GET /api/v1/jobs/:slug/copies?offset=3000000000` answers **500**. Verified on production:

```
offset=0           -> 200
offset=3000000000  -> 500
```

Fiber's `QueryInt` is a plain `strconv.Atoi`, so a 64-bit build accepts the value; the endpoint
then converts it to the `int32` the Postgres `int4` param needs, which wraps it **negative**, and
Postgres rejects a negative `OFFSET`. Every other list endpoint serves an empty page for the same
request, because they read pagination through the shared helper — whose doc comment names this
exact overflow. This one call site re-implemented the parse and left the clamp out.

It is a public, unauthenticated endpoint, so anyone can reach it.

## What Changes

- `JobCopies` reads its pagination through the shared helper. `?offset=3000000000` becomes an
  empty page, matching every other list endpoint.
- The helper generalises: `pageParamsMax(c, ceiling)` becomes `pageParamsBounded(c, fallback,
  ceiling)`, so a caller with its own default (copies pages 50 with a 200 cap) can use it instead
  of hand-rolling. `pageParams` and the two existing callers delegate to it unchanged.
- **A test now enforces the rule** rather than a comment: it scans the package for any file
  reading the offset query param outside the helper, and derives its population from behaviour so
  a new paginated endpoint is enrolled by existing. It fails on the current code.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-cluster-copies`: the endpoint's pagination bounds become part of its contract — an offset
  past the end is an empty page, not an error.

## Impact

- `internal/handler/copies.go` — two lines become one.
- `internal/handler/handler.go` — the helper's signature and its doc comment.
- `internal/handler/inbox.go`, `me_tracking.go` — the renamed helper, behaviour unchanged.
- No migration, no query change, no frontend change.
