// Catalogue-walk capacity suite: how fast can this hardware be drained?
//
// This models the extraction pattern production actually sees (measured
// 2026-08-21), not a browsing user. A catalogue walker works in two stages:
//
//   1. PROBE    GET /api/v1/jobs/search?company_slug=X&sort=created_at&order=desc&limit=1
//               "does this employer have anything, and how fresh is it" — cheap,
//               ~123 KB is the ordinary search's cost at limit=100 but this asks
//               for one row.
//   2. EXTRACT  GET /api/v1/agent/jobs/search?company_slug=X&limit=100
//                   &description_format=text&offset=0,100,200...
//               every posting with its full body. This is the expensive half:
//               it rehydrates each hit's description from Postgres (~833 KB and
//               ~1.3 s at limit=100 — internal/handler/public_read_limit.go).
//
// One iteration walks ONE company end to end, so a scenario's arrival `rate` is
// companies per second and the summary can project how long a full catalogue
// pass takes. pages.js answers "how many page renders per second"; this answers
// "how fast does the catalogue leave the building".
//
// Run it from the prod host against a loopback API port. See README.md.

import http from 'k6/http';
import exec from 'k6/execution';
import { check } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { BASE_URL, IS_LOCAL, USER_AGENT } from './config.js';

const env = (k, fallback) => (__ENV[k] !== undefined && __ENV[k] !== '' ? __ENV[k] : fallback);

// A knob that has to be a positive number, refused rather than coerced. `30s`
// reads as a perfectly reasonable duration and yields NaN, which reaches k6 as
// `duration: "NaNs"` and a preAllocatedVUs of NaN — so the run dies complaining
// about its options and says nothing about the variable that was mistyped.
const posNum = (k, fallback) => {
  const n = Number(env(k, fallback));
  if (!Number.isFinite(n) || n <= 0) {
    throw new Error(`${k} must be a positive number, got "${__ENV[k]}"`);
  }
  return n;
};

// This profile drives an origin as hard as it will go on its most expensive
// endpoint. Its own latch, with no localhost exemption — the intended target IS
// a loopback port on the prod host, which shares CPU, page cache, Postgres and
// Meilisearch with the colour serving live traffic.
if (env('FORCE_SCRAPER', '') !== '1') {
  throw new Error(
    `scraper.js saturates the full-description search on purpose — set FORCE_SCRAPER=1. ` +
      `Point it at the IDLE colour's API port, never at the live origin.`,
  );
}

// Companies per second, one step per value, run back to back. Each step is
// tagged so the knee is a number in the summary rather than a slope to eyeball.
const STEPS = env('SCRAPER_STEPS', '1,2,5,10,20')
  .split(',')
  .map((s) => Number(s.trim()))
  .filter((n) => Number.isFinite(n) && n > 0);
if (STEPS.length === 0) throw new Error('SCRAPER_STEPS parsed to nothing');

const STEP_SEC = posNum('SCRAPER_STEP_SEC', 30);

// How many company slugs to pull in setup(). The walk cycles through them; a
// pool smaller than the catalogue is fine and deliberate — this measures serving
// cost, and a warm slug costs the same as a cold one once Meili holds the whole
// index resident (it does: RssFile ≈ usedDatabaseSize on prod).
const SLUG_POOL = posNum('SCRAPER_SLUG_POOL', 300);

// How deep the walker paginates one employer before moving on. Real extraction
// was seen going to offset=1400. Capping it keeps a single mega-employer from
// dominating a step, which would measure that company rather than the host.
const MAX_PAGES = posNum('SCRAPER_MAX_PAGES', 5);

// Page size the extractor asks for. 100 is what the observed clients use and is
// the size the endpoint's cost was measured at.
const PAGE_SIZE = posNum('SCRAPER_PAGE_SIZE', 100);

// Seconds one company walk is expected to occupy a VU, used to size the VU pool
// (Little's law: concurrency = arrival rate x service time). Measured ~9 s at
// 20 companies/s against a loaded host with MAX_PAGES=5; 14 leaves room for the
// target slowing under its own load, which is exactly when the pool must not be
// the thing that runs out. Raise it, not the step duration, if a run reports
// dropped iterations.
const VU_SECONDS = posNum('SCRAPER_VU_SECONDS', 14);

// A walker that presents a distinct address per VU gets its own rate-limit
// budget, so the run measures the hardware. Left off, every VU shares one key.
//
// This is load-bearing and easy to get wrong, so it is explicit rather than
// implicit. cmd/server trusts X-Real-IP from a peer inside ratelimit.TrustedCIDRs
// (loopback is one), and internal/ratelimit.trusted then counts the CLAIMED
// address — a claimed public address is a stranger and is limited normally,
// while claiming nothing from loopback is exempt entirely. So:
//
//   SCRAPER_SPOOF_IP=1  each VU is its own caller → limiter effectively out of
//                       the way, the run reports what the box can serve.
//   SCRAPER_SPOOF_IP=0  from loopback the caller is trusted and exempt anyway;
//                       from off-host it is one limited caller and the run
//                       reports agentSearchPerMinute (300/min = 5 r/s), which is
//                       a real answer to a DIFFERENT question.
const SPOOF_IP = env('SCRAPER_SPOOF_IP', '1') === '1';

// Postings actually pulled out, and the bytes they left as. Rate-per-second on
// these two is the answer to "how fast does the catalogue drain".
const jobsHarvested = new Counter('scraper_jobs_harvested');
const bytesHarvested = new Counter('scraper_bytes_harvested');
const companiesWalked = new Counter('scraper_companies_walked');
// Wall time for one company end to end (probe + every extract page).
const walkDuration = new Trend('scraper_company_walk_ms', true);

export const options = {
  scenarios: buildScenarios(),
  thresholds: thresholds(),
  // The bodies ARE the measurement: walk() reads res.body.length for MiB/s and
  // res.json('data') for the posting count, so they have to be kept.
  discardResponseBodies: false,
};

function buildScenarios() {
  const scenarios = {};
  let start = 0;
  for (const rate of STEPS) {
    // A walk holds its VU for the whole probe+extract chain, which lengthens as
    // the target slows. Too few VUs and k6 misses the rate, then the shortfall
    // reads as the target's ceiling instead of the generator's.
    //
    // Preallocate the whole envelope rather than leaning on maxVUs: k6 allocates
    // past preAllocatedVUs LAZILY, and the allocation itself is slow enough that
    // iterations get dropped while waiting for it. The first run of this suite
    // lost 40 iterations at 20/s with 140 preallocated and 280 available — it
    // never ran out of headroom, it ran out of *ready* VUs. Budget seconds per
    // walk (SCRAPER_VU_SECONDS) times the arrival rate, which is Little's law.
    const preAllocatedVUs = Math.max(20, Math.ceil(rate * VU_SECONDS));
    scenarios[`walk_${rate}cps`] = {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration: `${STEP_SEC}s`,
      preAllocatedVUs,
      maxVUs: preAllocatedVUs * 2,
      startTime: `${start}s`,
      gracefulStop: '10s',
      exec: 'walk',
      tags: { step: String(rate) },
    };
    // A gap so a queue built at one rate drains before the next step begins;
    // without it every step inherits the previous backlog and the knee reads
    // earlier than it is.
    start += STEP_SEC + 15;
  }
  return scenarios;
}

function thresholds() {
  // This is a measurement, not a gate — every step past the ceiling is SUPPOSED
  // to degrade. k6 only prints a sub-metric that some threshold references, so
  // these unfailable entries exist to force the per-step breakdown into the
  // summary. Read the numbers, ignore the pass/fail.
  const t = {};
  for (const rate of STEPS) {
    t[`http_req_duration{step:${rate}}`] = ['p(95)>=0'];
    t[`http_req_failed{step:${rate}}`] = ['rate<=1'];
    t[`scraper_jobs_harvested{step:${rate}}`] = ['count>=0'];
  }
  // k6 materializes a sub-metric only for tags a threshold NAMES. Tagging
  // requests with `stage` and stopping there produced no probe-vs-extract split
  // at all — the tag was recorded and then had nowhere to land. These two lines
  // are what make the split exist.
  t['http_req_duration{stage:probe}'] = ['p(95)>=0'];
  t['http_req_duration{stage:extract}'] = ['p(95)>=0'];
  // Iterations k6 could not start for want of a free VU. Non-zero invalidates
  // the step: the generator fell behind, so the number describes the test.
  t['dropped_iterations'] = [{ threshold: 'count<1', abortOnFail: false }];
  return t;
}

// headers builds the per-VU request headers. The spoofed address is derived from
// the VU id so it is stable within a VU (one caller, one budget) and distinct
// across them. 203.0.113.0/24 is TEST-NET-3 (RFC 5737) — documentation-only
// space that can never collide with a real caller's key in Redis.
function headers() {
  const h = { 'User-Agent': USER_AGENT };
  if (SPOOF_IP) {
    const id = exec.vu.idInTest;
    h['X-Real-IP'] = `203.0.113.${id % 256}`;
    // Distinct /24s once the VU count passes 256, so a big run does not fold
    // sixty walkers back onto one budget.
    if (id > 255) h['X-Real-IP'] = `198.51.100.${id % 256}`;
  }
  return h;
}

// setup runs once: collect a pool of real company slugs and the catalogue's own
// size, so the summary can project a full pass instead of guessing at it.
export function setup() {
  const slugs = [];
  let total = 0;
  const perPage = 100;
  for (let offset = 0; slugs.length < SLUG_POOL; offset += perPage) {
    const res = http.get(`${BASE_URL}/api/v1/companies?limit=${perPage}&offset=${offset}`, {
      headers: { 'User-Agent': USER_AGENT },
    });
    if (res.status !== 200) {
      console.warn(`company listing failed (status ${res.status}) at offset ${offset} — pool is short`);
      break;
    }
    total = res.json('meta.total') || total;
    const rows = res.json('data') || [];
    if (rows.length === 0) break;
    for (const row of rows) if (row.slug) slugs.push(row.slug);
  }

  if (slugs.length === 0) {
    throw new Error(`no company slugs resolved from ${BASE_URL} — nothing to walk`);
  }

  console.log(
    `target=${BASE_URL} local=${IS_LOCAL} slugs=${slugs.length} catalogue=${total || 'unknown'} ` +
      `steps=${STEPS.join(',')} companies/s stepSec=${STEP_SEC} spoofIP=${SPOOF_IP}`,
  );
  return { slugs, total };
}

// walk extracts one company: probe for existence and freshness, then page
// through its full postings. Returns nothing — the counters are the output.
export function walk(data) {
  // Round-robin, not random, so two runs of the same steps stay comparable.
  const slug = data.slugs[exec.scenario.iterationInTest % data.slugs.length];
  const started = Date.now();
  const h = headers();

  // NOTE: do NOT tag `step` here. k6 applies each scenario's own `tags` to every
  // metric emitted inside it, and there is no `exec.scenario.tags` to read it
  // back from — passing one explicitly overwrites the scenario tag with whatever
  // you computed, which silently empties every `{step:N}` sub-metric and makes
  // the per-step table read as all zeros. `stage` is safe: it is a new dimension,
  // not a collision.

  // Stage 1 — the cheap probe.
  const probe = http.get(
    `${BASE_URL}/api/v1/jobs/search?company_slug=${encodeURIComponent(slug)}` +
      `&sort=created_at&order=desc&limit=1&offset=0`,
    { headers: h, tags: { stage: 'probe' } },
  );
  check(probe, { 'probe 200': (r) => r.status === 200 });
  bytesHarvested.add(probe.body ? probe.body.length : 0);

  if (probe.status !== 200) {
    walkDuration.add(Date.now() - started);
    companiesWalked.add(1);
    return;
  }

  // How many postings this employer has, per the probe's own meta. An employer
  // with nothing is dropped here — that is the walker's whole reason for
  // probing first, and skipping the check would turn every empty company into a
  // pointless full-description request and understate the real load mix.
  const available = probe.json('meta.total') || 0;
  if (available === 0) {
    walkDuration.add(Date.now() - started);
    companiesWalked.add(1);
    return;
  }

  // Stage 2 — the expensive extraction, paged until the employer runs out or
  // the depth cap is reached.
  const pages = Math.min(MAX_PAGES, Math.ceil(available / PAGE_SIZE));
  for (let page = 0; page < pages; page++) {
    const res = http.get(
      `${BASE_URL}/api/v1/agent/jobs/search?company_slug=${encodeURIComponent(slug)}` +
        `&limit=${PAGE_SIZE}&offset=${page * PAGE_SIZE}&description_format=text`,
      { headers: h, tags: { stage: 'extract' } },
    );
    // A 429 here is a finding, not an error: it means the run is measuring the
    // limiter rather than the hardware. Counted, then the walk stops — a
    // throttled caller does not keep paging.
    if (res.status === 429) {
      check(res, { 'not throttled': () => false });
      break;
    }
    check(res, { 'extract 200': (r) => r.status === 200 });
    if (res.status !== 200) break;

    bytesHarvested.add(res.body ? res.body.length : 0);
    const rows = res.json('data') || [];
    jobsHarvested.add(rows.length);
    if (rows.length < PAGE_SIZE) break; // employer exhausted
  }

  walkDuration.add(Date.now() - started);
  companiesWalked.add(1);
}

// handleSummary prints the per-step breakdown and the drain totals.
//
// It is deliberately self-contained. Returning ANYTHING from handleSummary
// replaces k6's default summary wholesale, so a run that only printed the
// totals silently threw away the per-step p95 and error rates — the numbers the
// thresholds exist to surface. Rebuilding them here, rather than importing
// jslib's textSummary, keeps the suite runnable on a prod host with no egress.
//
// The raw summary is also written to disk: whatever this formatter forgets to
// print is still recoverable without rerunning the load.
export function handleSummary(summary) {
  const m = summary.metrics;
  const secs = (summary.state && summary.state.testRunDurationMs / 1000) || 0;
  const jobs = (m.scraper_jobs_harvested && m.scraper_jobs_harvested.values.count) || 0;
  const bytes = (m.scraper_bytes_harvested && m.scraper_bytes_harvested.values.count) || 0;
  const companies = (m.scraper_companies_walked && m.scraper_companies_walked.values.count) || 0;
  const dropped = (m.dropped_iterations && m.dropped_iterations.values.count) || 0;

  const num = (v, digits = 0) => (typeof v === 'number' ? v.toFixed(digits) : '—');
  const lines = ['', '=== per step (companies/s imposed) ===', ''];
  lines.push('  step    p50 ms    p95 ms    max ms   fail%   postings   dropped');

  for (const rate of STEPS) {
    const dur = m[`http_req_duration{step:${rate}}`];
    const failed = m[`http_req_failed{step:${rate}}`];
    const harvest = m[`scraper_jobs_harvested{step:${rate}}`];
    const v = (dur && dur.values) || {};
    lines.push(
      `  ${String(rate).padEnd(6)}` +
        `${num(v.med).padStart(8)}  ${num(v['p(95)']).padStart(8)}  ${num(v.max).padStart(8)}` +
        `${num(failed && failed.values.rate * 100, 1).padStart(8)}` +
        `${num(harvest && harvest.values.count).padStart(11)}` +
        `${(rate === STEPS[STEPS.length - 1] ? String(dropped) : '—').padStart(10)}`,
    );
  }

  // A non-zero count means k6 could not start iterations for want of a free VU:
  // the generator fell behind, so those steps describe the test and not the
  // target. It is reported against the run, not per step — k6 does not tag it.
  if (dropped > 0) {
    lines.push('', `  ⚠ dropped_iterations=${dropped} — raise SCRAPER_STEP_SEC or the VU headroom and rerun`);
  }

  // Probe vs extract: which half of the walk the time actually goes into.
  const probe = m['http_req_duration{stage:probe}'];
  const extract = m['http_req_duration{stage:extract}'];
  if (probe || extract) {
    lines.push('', '=== by stage (whole run) ===', '');
    for (const [label, sm] of [['probe', probe], ['extract', extract]]) {
      const v = (sm && sm.values) || {};
      lines.push(
        `  ${label.padEnd(9)}p50 ${num(v.med).padStart(6)} ms   p95 ${num(v['p(95)']).padStart(7)} ms   ` +
          `max ${num(v.max).padStart(7)} ms   n=${num(v.count)}`,
      );
    }
  }

  const perSec = secs > 0 ? companies / secs : 0;
  const catalogue = (summary.setup_data && summary.setup_data.total) || 0;
  const fullPassHours = perSec > 0 && catalogue > 0 ? catalogue / perSec / 3600 : 0;

  lines.push(
    '',
    '=== catalogue drain (whole-run average, all steps blended) ===',
    `  companies walked   ${companies} (${perSec.toFixed(2)}/s)`,
    `  postings extracted ${jobs} (${(secs > 0 ? jobs / secs : 0).toFixed(1)}/s)`,
    `  bytes extracted    ${(bytes / 1048576).toFixed(1)} MiB (${(bytes / 1048576 / (secs || 1)).toFixed(2)} MiB/s)`,
    catalogue
      ? `  full catalogue     ${catalogue} companies → ${fullPassHours.toFixed(1)} h at this average rate`
      : `  full catalogue     size unknown (companies listing did not report meta.total)`,
    '',
    '  (blended across every step, so it understates the best sustained rate —',
    '   read it next to the per-step table, not instead of it)',
    '',
  );

  return {
    stdout: lines.join('\n'),
    'scraper-summary.json': JSON.stringify(summary, null, 2),
  };
}
