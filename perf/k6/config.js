// Shared configuration for the SSR page performance suite.
//
// Everything is env-overridable so the SAME script runs against a local
// `make up` stack (the default) or any remote origin (staging, prod) with no
// code changes — you only swap env vars.
//
//   PERF_BASE_URL origin that fronts BOTH the SSR pages and /api
//                 local: http://localhost:8090 (default) — prod: https://freehire.me
//   ALLOW_NONLOCAL=1   REQUIRED to target any non-localhost origin (a deliberate
//                      safety latch so a stray PERF_BASE_URL can't hammer prod)
//   PROFILE       'smoke' (default, isolated per-page) | 'load' (blended stress)
//   PERF_VUS      virtual users per scenario   (profile default otherwise)
//   PERF_DURATION duration per scenario        (load profile)
//
// NOTE: custom env vars are deliberately NOT prefixed `K6_` — k6 reserves that
// prefix for its own native options (e.g. K6_VUS would create a rogue "default"
// scenario and ignore ours), so we use PERF_* instead.
//   MAX_RPS       global request/sec ceiling   (0 = unlimited; auto-caps off-local)
//   QA_EMAIL / QA_PASSWORD   credentials the authed scenarios log in with (local)
//   AUTH_COOKIE   a full `hire_token=...` value to reuse instead of logging in
//                 (preferred against prod — no password in env, no /auth/login hit)

const env = (k, fallback) => (__ENV[k] !== undefined && __ENV[k] !== '' ? __ENV[k] : fallback);

// The local stack fronts SSR + /api behind one nginx origin, so a single base
// URL covers both the login call and every page navigation — exactly the
// same-origin shape the browser sees (the Lax auth cookie rides along). Prod is
// identical in shape: freehire.me fronts both, so only the origin changes.
export const BASE_URL = env('PERF_BASE_URL', 'http://localhost:8090');

// Targeting anything other than localhost is a deliberate act, never an env
// typo. This latch runs at init (top of every VU + setup), so a wrong
// PERF_BASE_URL aborts the whole run before a single request is fired at prod.
export const IS_LOCAL = /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?(\/|$)/.test(BASE_URL);
if (!IS_LOCAL && env('ALLOW_NONLOCAL', '') !== '1') {
  throw new Error(
    `Refusing to run against non-local origin "${BASE_URL}". All target pages are ` +
      `idempotent GETs, but this is still live traffic — set ALLOW_NONLOCAL=1 to confirm.`,
  );
}

// Think-time (seconds) between page views. Models a real user reading a page,
// and stops a VU tight-looping into a request storm when a target fails fast.
// Zero under an arrival-rate executor, where pacing is the scenario's job and a
// sleep only inflates the VU count needed to sustain the rate.
export const THINK = Number(env('THINK', env('PROFILE', 'smoke') === 'saturation' ? 0 : 1));

export const PROFILE = env('PROFILE', 'smoke');
// The blended 'load' profile can generate real pressure; block it off-local
// unless explicitly forced, so a prod smoke can't accidentally become a stress test.
if (!IS_LOCAL && PROFILE === 'load' && env('FORCE_LOAD', '') !== '1') {
  throw new Error(`PROFILE=load against a non-local origin needs FORCE_LOAD=1 (this stresses live infra).`);
}

// 'saturation' answers what the other profiles structurally cannot: how many
// requests per second does this deployment accept before it stops keeping up?
// `load` is a CLOSED model — a VU waits for its response before sending the next,
// so a slowing target slows the test with it and the run converges on whatever
// the server can serve. It cannot overrun anything, which is the failure being
// reproduced: arrivals outrunning accepts until the kernel drops SYNs.
//
// Steps rather than a ramp, each tagged with its rate, so the breaking point is a
// number in the summary instead of a slope to eyeball.
//
// Its own latch, with no localhost exemption: the intended target IS a localhost
// port on the prod host (the idle colour), which shares CPU, page cache and
// Postgres with the live one.
export const SATURATION_STEPS = env('SATURATION_STEPS', '25,50,100,200,400')
  .split(',')
  .map((s) => Number(s.trim()))
  .filter((n) => Number.isFinite(n) && n > 0);
export const SATURATION_STEP_SEC = Number(env('SATURATION_STEP_SEC', 30));
if (PROFILE === 'saturation') {
  if (env('FORCE_SATURATION', '') !== '1') {
    throw new Error(
      `PROFILE=saturation drives a target past its capacity on purpose — set FORCE_SATURATION=1. ` +
        `Point it at an idle colour (http://127.0.0.1:8084), never at the live origin.`,
    );
  }
  if (SATURATION_STEPS.length === 0) throw new Error(`SATURATION_STEPS parsed to nothing`);
}

// Global request-rate ceiling. Off-local defaults to a gentle cap so a smoke
// against prod stays polite; local is unlimited. Saturation is uncapped whatever
// the origin — a ceiling would clamp the rate the profile exists to impose, and
// the run would report the cap as the target's capacity. FORCE_SATURATION is the
// safety there.
export const MAX_RPS = Number(env('MAX_RPS', PROFILE === 'saturation' || IS_LOCAL ? 0 : 20));

// Identifies suite traffic in access logs / analytics so it's separable from
// real users (and honest about being a bot to any WAF).
export const USER_AGENT = env('USER_AGENT', 'freehire-perf-k6/1.0 (+load-test)');

// The QA account is a local-dev fixture, so it's only the default off-nothing
// when targeting localhost. Against any remote origin, creds must be passed
// explicitly (or, preferably, AUTH_COOKIE) — otherwise the authed scenarios stay
// anonymous rather than firing a doomed login at prod.
export const CREDS = {
  email: env('QA_EMAIL', IS_LOCAL ? 'qa@freehire.local' : ''),
  password: env('QA_PASSWORD', IS_LOCAL ? 'tracking-qa-2026' : ''),
};

// A pre-captured `hire_token=...` cookie. When set, the authed scenarios reuse
// it verbatim and setup() skips the login POST — the safe path for prod, where
// the local QA account doesn't exist and you don't want a password in env.
export const AUTH_COOKIE = env('AUTH_COOKIE', '');

// A company slug for the company-card scenario, bypassing the live lookup in
// setup(). Required when the target does not front /api (an SSR process
// addressed directly by port), optional everywhere else.
export const COMPANY_SLUG = env('PERF_COMPANY_SLUG', '');

// The two auth modes every page is exercised in. `anon` sends no cookie;
// `authed` attaches the `hire_token` session, which makes the layout resolve
// /me and the pages render personalized (heavier) chrome — measurably more data.
export const MODES = ['anon', 'authed'];

// The representative "worst-case" filtered feed the homepage renders. This is a
// product-knowledge choice: it should mirror a realistic heavy user query —
// full-text search + several ANDed facets + a sort — over values that actually
// return a populated result set (an empty result is a cheap fast path and hides
// the real cost). Facet param names are the ones the search index accepts
// (internal/search/query_filter.go).
export const FILTER_QUERY = env(
  'FILTER_QUERY',
  'q=engineer&work_mode=remote&seniority=senior&skills=go&sort=posted_at',
);

// A representative filtered companies view (?q + sidebar facets). Params match
// GET /api/v1/companies (internal/handler/companies.go).
export const COMPANY_FILTER_QUERY = env('COMPANY_FILTER_QUERY', 'regions=europe&company_type=startup');

// The pages under test, in the order the user cares about: the homepage job
// feed, that same feed under a heavy filter, the companies catalogue, and a
// single company card (its :slug is resolved from live data in setup()).
export const PAGES = {
  home: { label: 'homepage job feed', path: '/' },
  homeFiltered: { label: 'filtered job feed', path: `/?${FILTER_QUERY}` },
  companies: { label: 'companies catalogue', path: `/companies?${COMPANY_FILTER_QUERY}` },
  companyCard: { label: 'company card (streamed jobs)', path: '/companies/__SLUG__' },
};

// Per-page p95 latency budgets (ms) and a global error budget. Starting SLOs —
// tune per hardware/data once you have a baseline. The company card is looser
// because it streams its job list in the same response; the filtered feed is
// looser because it hits Meili full-text + facets.
const P95 = {
  home: Number(env('SLO_HOME', 1500)),
  homeFiltered: Number(env('SLO_HOME_FILTERED', 2500)),
  companies: Number(env('SLO_COMPANIES', 2000)),
  companyCard: Number(env('SLO_COMPANY_CARD', 3000)),
};

export function thresholds() {
  // Saturation is a measurement, not a gate: every step past the ceiling is
  // SUPPOSED to fail. It needs a per-step breakdown instead, and k6 only prints a
  // sub-metric some threshold references — hence these unfailable thresholds.
  // Read the numbers, ignore the pass/fail.
  if (PROFILE === 'saturation') {
    const t = {};
    for (const rate of SATURATION_STEPS) {
      t[`http_req_duration{step:${rate}}`] = ['p(95)>=0'];
      t[`http_req_failed{step:${rate}}`] = ['rate<=1'];
    }
    // Iterations k6 could not start for want of a free VU: the generator itself
    // fell behind, so those numbers describe the test, not the target. Non-zero
    // invalidates the step — raise preAllocatedVUs and rerun.
    t['dropped_iterations'] = [{ threshold: 'count<1', abortOnFail: false }];
    return t;
  }

  const t = {
    http_req_failed: [{ threshold: 'rate<0.02', abortOnFail: false }],
    checks: ['rate>0.98'],
  };
  for (const page of Object.keys(PAGES)) {
    for (const auth of MODES) {
      t[`http_req_duration{page:${page},auth:${auth}}`] = [`p(95)<${P95[page]}`];
    }
  }
  return t;
}

// buildScenarios wires one k6 scenario per (page × auth) so metrics split cleanly
// by tag. `smoke` runs them sequentially (staggered startTime, low VUs) to keep
// per-page latency uncontended — the right shape for regression comparison and
// for a polite prod smoke. `load` runs them all at once under a ramping profile.
export function buildScenarios() {
  const pages = Object.keys(PAGES);
  const scenarios = {};

  // saturation: one constant-arrival-rate step per target rate, run back to back.
  // Anonymous only — the traffic this models is crawlers and cold visitors, and
  // an authed run would measure a session cookie's cost rather than the ceiling.
  if (PROFILE === 'saturation') {
    let start = 0;
    for (const rate of SATURATION_STEPS) {
      // Headroom: at the ceiling each iteration holds its VU longer, and too few
      // VUs makes k6 miss the rate and blame the target for its own shortfall.
      const preAllocatedVUs = Math.max(50, rate * 4);
      scenarios[`saturation_${rate}rps`] = {
        executor: 'constant-arrival-rate',
        rate,
        timeUnit: '1s',
        duration: `${SATURATION_STEP_SEC}s`,
        preAllocatedVUs,
        maxVUs: preAllocatedVUs * 2,
        startTime: `${start}s`,
        gracefulStop: '5s',
        exec: 'run',
        tags: { step: String(rate), auth: 'anon' },
      };
      // A gap between steps so a queue built at one rate drains before the next
      // begins — otherwise every step inherits the previous one's backlog and
      // the ceiling reads lower than it is.
      start += SATURATION_STEP_SEC + 10;
    }
    return scenarios;
  }

  if (PROFILE === 'load') {
    const vus = Number(env('PERF_VUS', 15));
    const dur = env('PERF_DURATION', '1m');
    for (const page of pages) {
      for (const auth of MODES) {
        scenarios[`${page}_${auth}`] = {
          executor: 'ramping-vus',
          startVUs: 0,
          stages: [
            { target: vus, duration: '15s' },
            { target: vus, duration: dur },
            { target: 0, duration: '10s' },
          ],
          exec: 'run',
          tags: { page, auth },
        };
      }
    }
    return scenarios;
  }

  // smoke: isolate each scenario in its own time window so their latencies don't
  // contend. Off-local we clamp VUs low regardless of PERF_VUS — the rps cap and a
  // couple of VUs are plenty to measure latency without pressuring prod.
  const vus = IS_LOCAL ? Number(env('PERF_VUS', 2)) : Math.min(Number(env('PERF_VUS', 2)), 2);
  const windowSec = Number(env('PERF_WINDOW_SEC', 15));
  let i = 0;
  for (const page of pages) {
    for (const auth of MODES) {
      scenarios[`${page}_${auth}`] = {
        executor: 'constant-vus',
        vus,
        duration: `${windowSec}s`,
        startTime: `${i * windowSec}s`,
        gracefulStop: '3s',
        exec: 'run',
        tags: { page, auth },
      };
      i++;
    }
  }
  return scenarios;
}
