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
PERF_BASE_URL=https://freehire.dev \
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

## Key knobs

| env                    | default                       | purpose                                        |
| ---------------------- | ----------------------------- | ---------------------------------------------- |
| `PERF_BASE_URL`        | `http://localhost:8090`       | target origin (fronts SSR + `/api`)            |
| `ALLOW_NONLOCAL`       | —                             | must be `1` for any non-localhost origin       |
| `PROFILE`              | `smoke`                       | `smoke` (isolated) or `load` (blended)         |
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
