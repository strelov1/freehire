# Page performance suite (k6)

End-to-end SSR performance for freehire's most important pages. Each request
renders a real page through the full server stack — SvelteKit SSR → `serverApi`
→ Go API → Postgres/Meili — so the numbers reflect what a user's first byte and
full HTML actually cost, including the company card's streamed job list.

Pages covered, each **anonymous and authenticated**:

| scenario key   | page                              | route                         |
| -------------- | --------------------------------- | ----------------------------- |
| `home`         | homepage job feed (jobview)       | `/`                           |
| `homeFiltered` | job feed under a heavy filter     | `/?q=…&work_mode=…&…`         |
| `companies`    | companies catalogue               | `/companies?…`                |
| `companyCard`  | one company card (streamed jobs)  | `/companies/<slug>`           |

The authenticated variants attach the `hire_token` session cookie, so the layout
resolves `/me` and the pages render personalized, heavier chrome. The suite
records `page_body_bytes{auth:…}` so you can see anon vs authed payload size.

## Prerequisites

- [k6](https://k6.io/docs/get-started/installation/) (`brew install k6`)
- A running target. Locally: `make up` (SSR + API behind nginx on `:8090`).

## Run locally (default)

```bash
k6 run perf/k6/pages.js
```

Defaults: `PERF_BASE_URL=http://localhost:8090`, `PROFILE=smoke`, authed via the
local QA account (`qa@freehire.local`). The `smoke` profile runs each
scenario in its own time window (staggered) so per-page latency doesn't contend.

## Run against prod (env swap only)

All target pages are idempotent GETs. Pointing at a non-local origin requires an
explicit `ALLOW_NONLOCAL=1` latch so a stray `PERF_BASE_URL` can never quietly load
prod. Prefer a real captured session cookie over prod credentials:

```bash
PERF_BASE_URL=https://freehire.me \
ALLOW_NONLOCAL=1 \
AUTH_COOKIE='<paste hire_token value from your browser>' \
MAX_RPS=20 \
k6 run perf/k6/pages.js
```

Off-local safety is automatic: VUs are clamped low, a global `MAX_RPS` ceiling
applies (default 20), traffic carries a `freehire-perf-k6` User-Agent, and the
`load` profile is refused unless `FORCE_LOAD=1`. Anonymous-only prod smoke: just
omit `AUTH_COOKIE`.

## Blended load test

```bash
PROFILE=load PERF_VUS=20 PERF_DURATION=2m k6 run perf/k6/pages.js
```

Runs all scenarios concurrently under a ramping-VUs profile (local by default;
`FORCE_LOAD=1` required off-local).

## Capacity test (`PROFILE=saturation`)

Answers "how many requests per second does this deployment accept before it stops
keeping up" — the question behind the recurring accept-queue incidents.

```bash
PROFILE=saturation FORCE_SATURATION=1 \
  PERF_BASE_URL=http://127.0.0.1:8084 \
  SATURATION_STEPS=25,50,100,200,400 SATURATION_STEP_SEC=30 \
  k6 run perf/k6/pages.js
```

**Why it is not `PROFILE=load`.** `load` is a closed model: a VU waits for its
response before sending the next request, so a target that slows down slows the
test with it, and the run settles at whatever the server can serve. That measures
latency under a fixed audience — it cannot overrun anything. The failure mode in
production is the opposite: arrivals outpace accepts until the kernel's accept
queue fills and drops SYNs. `saturation` is an open model — `constant-arrival-rate`
imposes the rate whether or not the target is coping.

Steps rather than a ramp, each one its own scenario tagged with its rate, so the
summary carries a clean p95 and error rate **per rate** and the ceiling comes out
as a number. Ten idle seconds between steps let a queue drain, so no step inherits
the previous one's backlog.

**Run it against an idle blue/green colour on the prod host, never the live
origin.** The idle colour has the same data, the same Postgres and the same Meili,
but no users — while still sharing CPU and page cache with the live colour, which
is why the profile has its own `FORCE_SATURATION` latch and ignores the
`IS_LOCAL` exemptions (the intended target is a localhost port *on prod*). Watch
the live colour's accept queue in another shell and stop if it moves:

```bash
watch -n1 'ss -ltn "sport = :8083 or sport = :8084"'
```

`dropped_iterations > 0` invalidates a step: k6 itself could not keep up, so that
step describes the load generator, not the target. Raise the VU headroom and rerun.

## Key knobs

| env                    | default                       | purpose                                        |
| ---------------------- | ----------------------------- | ---------------------------------------------- |
| `PERF_BASE_URL`        | `http://localhost:8090`       | target origin (fronts SSR + `/api`)            |
| `ALLOW_NONLOCAL`       | —                             | must be `1` for any non-localhost origin       |
| `PROFILE`              | `smoke`                       | `smoke` (isolated), `load` (blended), `saturation` (capacity) |
| `FORCE_SATURATION`     | —                             | must be `1` to run the capacity profile        |
| `SATURATION_STEPS`     | `25,50,100,200,400`           | request/sec steps, run back to back            |
| `SATURATION_STEP_SEC`  | `30`                          | seconds held at each step                      |
| `AUTH_COOKIE`          | —                             | reuse a `hire_token` instead of logging in     |
| `QA_EMAIL/QA_PASSWORD` | local QA account              | password login for the authed scenarios        |
| `MAX_RPS`              | `0` local / `20` off-local    | global request/sec ceiling                     |
| `FILTER_QUERY`         | `q=engineer&work_mode=…`      | the heavy feed filter (tune to your data)      |
| `COMPANY_FILTER_QUERY` | `regions=europe&company_type=startup` | companies-list facets                  |
| `SLO_*`                | see `config.js`               | per-page p95 latency budgets (ms)              |

## Reading results

- `http_req_duration{page:…,auth:…}` — per-page, per-mode latency (p95 gates in
  `thresholds()`).
- `http_req_failed` — connection/5xx rate (global gate `< 2%`).
- `page_body_bytes{auth:…}` — payload size; authed should exceed anon.
- `checks` — every page must return `200` + real HTML.

For `PROFILE=saturation`, read `http_req_duration{step:N}` and
`http_req_failed{step:N}` across steps: the ceiling is the last rate where the
error rate stays near zero and p95 has not yet turned the corner. Those
thresholds are deliberately unfailable — they exist only to make k6 print the
per-step breakdown, so ignore their pass/fail and read the numbers.
