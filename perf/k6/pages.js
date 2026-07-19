// SSR page performance suite for freehire's most important pages:
//   - the homepage job feed (jobview), plain and under a heavy filter
//   - the companies catalogue
//   - a single company card (streamed job list)
// Each is exercised anonymously AND authenticated (the authed path resolves /me
// and renders heavier, personalized chrome). See README.md to run.

import exec from 'k6/execution';
import { PAGES, MODES, PROFILE, BASE_URL, MAX_RPS, buildScenarios, thresholds } from './config.js';
import { resolveSession, pickCompanySlug, visit } from './helpers.js';

export const options = {
  scenarios: buildScenarios(),
  thresholds: thresholds(),
  // Global request-rate ceiling (0 = unlimited). Keeps prod smokes polite.
  ...(MAX_RPS > 0 ? { rps: MAX_RPS } : {}),
};

// setup runs once. It establishes the authed session and resolves a real
// company slug, then hands both to every VU iteration — so the login and the
// company lookup happen a single time, not per request.
export function setup() {
  const cookie = resolveSession();
  const slug = pickCompanySlug();
  console.log(
    `target=${BASE_URL} profile=${PROFILE} authed=${cookie ? 'yes' : 'no'} companySlug=${slug || '(none)'}`,
  );
  return { cookie, slug };
}

// One entry point for every scenario. k6 tells us which scenario is running via
// exec.scenario.name (e.g. "companyCard_authed"); we split it into page + auth
// and dispatch — no per-scenario boilerplate.
export function run(data) {
  const name = exec.scenario.name;
  const sep = name.lastIndexOf('_');
  const page = name.slice(0, sep);
  const auth = name.slice(sep + 1);
  if (!PAGES[page] || !MODES.includes(auth)) throw new Error(`unmapped scenario ${name}`);
  visit(page, PAGES[page].path, auth, data);
}
