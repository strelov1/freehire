import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';
import { BASE_URL, USER_AGENT, AUTH_COOKIE, CREDS, THINK } from './config.js';

// Body size of every rendered page, tagged by {page, auth}. This makes the
// "authed pulls more data" claim measurable: in the summary, authed body bytes
// come out heavier than anon because the layout resolves /me and the pages add
// signed-in chrome and personalization.
const bodyBytes = new Trend('page_body_bytes');

// Default request params: identify suite traffic and don't chase redirects
// silently — an SSR page that 301/302s (e.g. /jobs → /) is a signal we want to
// see, not swallow.
function params(pageKey, auth, cookie) {
  const headers = { 'User-Agent': USER_AGENT };
  if (auth === 'authed' && cookie) headers['Cookie'] = cookie;
  return { headers, tags: { page: pageKey, auth }, redirects: 5 };
}

// resolveSession returns the `hire_token=...` cookie string used by authed
// scenarios. Preference order: a pre-captured AUTH_COOKIE (the prod-safe path),
// otherwise a one-shot password login against /api/v1/auth/login. Returns '' if
// no session could be established — callers then skip the authed path rather
// than silently measuring an anonymous request mislabeled as authed.
export function resolveSession() {
  if (AUTH_COOKIE) {
    // Accept either a bare token or a full `name=value` pair.
    return AUTH_COOKIE.includes('=') ? AUTH_COOKIE : `hire_token=${AUTH_COOKIE}`;
  }
  // No usable session source: don't hit /auth/login at all. This keeps anon-only
  // runs (notably prod smokes) from wasting a request — and, since a 401 counts
  // as a failed http_req, from polluting the error-rate threshold.
  if (!CREDS.email || !CREDS.password) return '';

  // A few attempts absorb a cold reverse-proxy at t=0 (status 0). A real 401
  // (bad creds) is terminal — don't retry it.
  let res;
  for (let i = 0; i < 3; i++) {
    res = http.post(
      `${BASE_URL}/api/v1/auth/login`,
      JSON.stringify({ email: CREDS.email, password: CREDS.password }),
      { headers: { 'Content-Type': 'application/json', 'User-Agent': USER_AGENT } },
    );
    if (res.status !== 0) break;
    sleep(1);
  }
  const jar = res.cookies['hire_token'];
  if (res.status !== 200 || !jar || jar.length === 0) {
    console.warn(
      `login failed (status ${res.status}) — authed scenarios will be skipped. ` +
        `On prod, pass AUTH_COOKIE instead of QA creds.`,
    );
    return '';
  }
  return `hire_token=${jar[0].value}`;
}

// pickCompanySlug pulls a real company slug from live data so the company-card
// scenario is never hardcoded/brittle. It prefers a company with open jobs so
// the streamed job list actually does work (the interesting latency path).
export function pickCompanySlug() {
  let res;
  for (let i = 0; i < 3; i++) {
    res = http.get(`${BASE_URL}/api/v1/companies?limit=50`, { headers: { 'User-Agent': USER_AGENT } });
    if (res.status === 200) break;
    sleep(1);
  }
  if (res.status !== 200) {
    console.warn(`could not list companies (status ${res.status}) — company-card scenarios skipped`);
    return '';
  }
  const rows = (res.json('data') || []);
  if (rows.length === 0) return '';
  const withJobs = rows.find((c) => (c.job_count || 0) > 0);
  return (withJobs || rows[0]).slug || '';
}

// visit renders one SSR page and records latency (tagged), body size, and
// content checks. `data` carries the resolved session cookie and company slug
// from setup(); a scenario with no usable session/slug is skipped, not faked.
export function visit(pageKey, path, auth, data) {
  if (auth === 'authed' && !data.cookie) return; // no session — don't mislabel anon as authed
  const url = BASE_URL + path.replace('__SLUG__', data.slug);
  if (path.includes('__SLUG__') && !data.slug) return; // no company to render

  const res = http.get(url, params(pageKey, auth, data.cookie));
  bodyBytes.add(res.body ? res.body.length : 0, { page: pageKey, auth });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'is html document': (r) => typeof r.body === 'string' && r.body.toLowerCase().includes('<!doctype html'),
    'body is non-trivial': (r) => r.body && r.body.length > 1000,
  });

  // Think-time between page views: realistic pacing, and a hard brake on the
  // tight-loop request storm that a fast-failing target would otherwise cause.
  sleep(THINK);
}
