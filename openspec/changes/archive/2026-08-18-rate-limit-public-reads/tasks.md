## 1. Carry the backend's numbers through the Throttler contract

- [x] 1.1 Introduce `ratelimit.Decision{Allowed, Limit, Remaining, ResetAfter,
      RetryAfter}` and change `Throttler.Allow` to return `(Decision, error)`;
      update `RedisThrottler.Allow` to populate every field from
      `redis_rate.Result` rather than discarding `Remaining`/`ResetAfter`
- [x] 1.2 Update the four test fakes in `internal/ratelimit` to the widened
      interface, keeping each one's existing behaviour (in-memory counter,
      always-error, hang-past-timeout, sub-second retry)
- [x] 1.3 `Middleware` sets `X-RateLimit-Limit`/`Remaining`/`Reset` on both the
      allowed and the refused path, keeps `Retry-After` and its one-second floor
      on refusal, and sets no headers when it fails open — with a test per branch,
      including that `Remaining` decreases across two calls

## 2. Do not limit our own front end

- [x] 2.1 Lift the proxy-trust CIDR list out of `cmd/server/main.go` into
      `internal/ratelimit` as the single definition, and have `cmd/server` read it
      from there so the two cannot drift
- [x] 2.2 `Middleware` returns `c.Next()` without consulting the throttler when the
      peer is loopback or private, setting no headers; test that a loopback caller
      far over the limit is never refused while an external one still is

## 3. Attach the two public-read budgets

- [x] 3.1 Add the budget constants and their two key prefixes in one place, sized
      from the measured peaks recorded in design.md (cheap 600/min, agent 300/min),
      each carrying a comment naming the peak it was derived from
- [x] 3.2 Attach the cheap limiter to EVERY public job and company read —
      `/jobs`, `/jobs/find`, `/jobs/search`, `/jobs/facets`, `/jobs/{slug}` and
      its `/copies` and `/apply-form`, `/jobs/{slug}/similar`, `/companies`,
      `/companies/subindustries`, `/companies/{slug}` and `/geo/cities` — keyed
      by `KeyByUserOrIP`. The sitemaps stay unlimited on purpose: they are the
      polite bulk path, nginx-cached, and throttling them pushes crawlers back
      onto the endpoints this change is protecting
- [x] 3.3 Attach the expensive limiter to `/agent/jobs/search` alone, with its own
      prefix; test that exhausting it leaves the ordinary search endpoint serving
- [x] 3.4 Log every refusal with the request path and user agent, so "who did this
      catch" is answerable later without adding instrumentation after the fact

## 4. Publish the ceiling

- [x] 4.1 Document the two budgets, the three headers and `Retry-After` in
      `web/static/openapi.yaml`, in the same filter-grammar section that already
      warns about ignored parameters — a limit nobody can read about is the defect
      this change exists to fix
- [x] 4.2 State in the schema that a loopback/internal caller is exempt only to the
      extent of saying the limits apply to public callers, without publishing the
      trust list itself

## 5. Verify against the case that prompted this

- [x] 5.1 A test asserting the agent-search budget admits more than the 184
      requests/minute the heaviest live consumer currently sends, so the change
      cannot silently cut off a caller it was designed to tolerate
- [x] 5.2 `go vet -tags=integration ./...` before pushing, and the full tagged
      suite for `internal/handler` and `internal/ratelimit` since behaviour changed
