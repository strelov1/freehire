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

## Catalogue drain test (`scraper.js`)

Answers a different question from `pages.js`: not "how many page renders per
second", but **"how fast can this hardware be emptied of its catalogue"**. It
replays the two-stage extraction pattern production measured on 2026-08-21 —
a cheap `jobs/search?company_slug=…&limit=1` probe, then paged
`agent/jobs/search?limit=100&description_format=text` over every employer that
has postings.

```bash
# on host-2. Resolve the IDLE colour FIRST — the API ports are blue :8081 /
# green :8082, and `hire-current` points at whichever is live.
readlink /opt/freehire/src/hire-current   # → hire-green ⇒ idle API is :8081

FORCE_SCRAPER=1 \
  PERF_BASE_URL=http://127.0.0.1:8081 \
  SCRAPER_STEPS=1,2,5,10,20 SCRAPER_STEP_SEC=30 \
  k6 run perf/k6/scraper.js
```

One iteration walks one company end to end, so a step's `rate` is **companies per
second**. The custom summary reports postings/s, MiB/s, and the projected hours
for a full pass over the live company count.

**The rate limiter is the trap here.** `agent/jobs/search` is capped at 300/min
(5 r/s) per caller and the cheap reads at 600/min, keyed by user-or-IP
(`internal/handler/public_read_limit.go`). A single-machine run therefore
measures the limiter, not the host, unless you say otherwise. `cmd/server` trusts
`X-Real-IP` from a peer inside `ratelimit.TrustedCIDRs`, and
`internal/ratelimit.trusted` then counts the **claimed** address — so
`SCRAPER_SPOOF_IP=1` (the default) gives each VU its own budget from
documentation-only address space (RFC 5737) and the run reports what the box can
serve. Set `SCRAPER_SPOOF_IP=0` to measure the throttled path instead — a real
answer to the different question "how fast can one outside client drain us".
A 429 during extraction fails the `not throttled` check rather than passing
silently, so a misconfigured run is visible in the summary instead of reading as
a low ceiling.

Same siting rule as `PROFILE=saturation`: point it at the **idle** colour's API
port on the prod host, never the live origin. It has its own `FORCE_SCRAPER`
latch with no localhost exemption for exactly that reason.

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
| `FORCE_SCRAPER`        | —                             | must be `1` to run `scraper.js`                |
| `SCRAPER_STEPS`        | `1,2,5,10,20`                 | companies/sec steps, run back to back          |
| `SCRAPER_STEP_SEC`     | `30`                          | seconds held at each step                      |
| `SCRAPER_SPOOF_IP`     | `1`                           | per-VU `X-Real-IP` so the limiter isn't what you measure |
| `SCRAPER_SLUG_POOL`    | `300`                         | company slugs pulled in `setup()`              |
| `SCRAPER_MAX_PAGES`    | `5`                           | extraction depth per employer (100/page)       |
| `SCRAPER_PAGE_SIZE`    | `100`                         | postings per extraction request                |
| `SCRAPER_VU_SECONDS`   | `14`                          | seconds budgeted per company walk; sizes the VU pool — raise THIS, not the step duration, when a run drops iterations |

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
