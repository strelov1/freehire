// Deep-offset probe: what does ONE request cost at a given offset?
//
// Not a load test, and deliberately not shaped like one. The question `pages.js` and
// `scraper.js` answer is "how much can this host take"; the question here is "how expensive
// is a single request the caller gets to price", which is the property the 2026-09-14 outage
// turned on. A crawler inside its 600/min rate budget took the catalogue offline because
// `LIMIT n OFFSET k` reads and discards k rows, and it chose k.
//
// ONE VU, strictly sequential, by construction. The whole point is to measure a single
// request in isolation — and against an origin whose Postgres is shared with the live colour,
// concurrency here would be a re-enactment rather than a measurement. Ten concurrent deep
// offsets is exactly what exhausted the pool.
//
//   # Resolve the IDLE colour FIRST. API ports: blue :8081, green :8082.
//   readlink /opt/freehire/src/hire-current   # -> hire-green  =>  idle API is :8081
//
//   FORCE_DEEP_OFFSET=1 PERF_BASE_URL=http://127.0.0.1:8081 k6 run perf/k6/deepoffset.js
//
// The latch has no localhost exemption, for the same reason `scraper.js`'s does not: the
// intended target IS a localhost port, on prod. Being local says nothing about being safe.
//
// After the pagination window ships, every offset past it answers 400 in about a millisecond
// and `deep_offset_refused` is the rate that proves it. Before it, the same run measures
// seconds per request and climbing — which is the before/after this script exists to record.

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { BASE_URL } from './config.js';

if (__ENV.FORCE_DEEP_OFFSET !== '1') {
  throw new Error(
    'deepoffset.js walks pagination deliberately deep against a real catalogue. Point it at ' +
      "the IDLE colour's API port on the prod host, never the live origin, then set " +
      'FORCE_DEEP_OFFSET=1 to confirm.',
  );
}

// The offsets the incident actually passed through, plus a shallow control. Each is its own
// tag, so the summary reads as a cost curve rather than one blended number — the curve IS the
// finding, because a flat one would mean OFFSET was not what hurt.
const OFFSETS = (__ENV.DEEP_OFFSETS || '0,1000,5000,9900,10000,20000,100000,179500')
  .split(',')
  .map((s) => Number(s.trim()))
  .filter((n) => Number.isInteger(n) && n >= 0);

const PAGE_SIZE = Number(__ENV.DEEP_OFFSET_LIMIT || 100);

const latency = new Trend('deep_offset_latency', true);
const refused = new Rate('deep_offset_refused');
const served = new Rate('deep_offset_served');

export const options = {
  scenarios: {
    walk: { executor: 'shared-iterations', vus: 1, iterations: OFFSETS.length, maxDuration: '20m' },
  },
  // Unfailable on purpose: the run exists to PRINT the per-offset breakdown, not to pass or
  // fail. Read the numbers, ignore the verdict — the same convention PROFILE=saturation uses.
  thresholds: Object.fromEntries(
    OFFSETS.map((o) => [`deep_offset_latency{offset:${o}}`, ['p(95)>=0']]),
  ),
};

export default function () {
  const offset = OFFSETS[__ITER % OFFSETS.length];
  const tags = { offset: String(offset) };

  const res = http.get(`${BASE_URL}/api/v1/jobs?limit=${PAGE_SIZE}&offset=${offset}`, {
    tags,
    // Generous, and not a timeout to tune: before the fix a single request at offset=179500
    // held a pooled connection for over two minutes, and cutting the client would hide
    // exactly the number this run is here to record. The server keeps working either way —
    // that is the defect, not an artefact of the probe.
    timeout: '180s',
  });

  latency.add(res.timings.duration, tags);
  refused.add(res.status === 400, tags);
  served.add(res.status === 200, tags);

  check(res, {
    // Not "is 200": past the window a 400 is the CORRECT answer, and asserting 200 would fail
    // the run precisely when the fix is working.
    'answered 200 or 400, never 5xx': (r) => r.status === 200 || r.status === 400,
  });
}

export function handleSummary(data) {
  const rows = OFFSETS.map((o) => {
    const t = data.metrics[`deep_offset_latency{offset:${o}}`];
    const r = data.metrics[`deep_offset_refused{offset:${o}}`];
    const ms = t && t.values ? t.values.avg : NaN;
    const wasRefused = r && r.values ? r.values.rate === 1 : false;
    return `  offset=${String(o).padStart(7)}  ${
      wasRefused ? 'REFUSED 400' : `${(ms / 1000).toFixed(3)}s`
    }`;
  });

  return {
    stdout: [
      '',
      'Cost of one /api/v1/jobs request, by offset:',
      ...rows,
      '',
      'A curve that climbs with the offset is the defect. A flat line that turns into',
      'REFUSED at the window is the fix.',
      '',
    ].join('\n'),
  };
}
