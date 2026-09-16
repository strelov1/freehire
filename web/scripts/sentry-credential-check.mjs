// Does this release's Sentry credential actually work?
//
// It exists because nothing else can answer that. `@sentry/sveltekit`'s own
// `vite/sourceMaps.js` wraps the upload in a bare `catch {}` that prints a warning and lets
// the build succeed, ABOVE `@sentry/vite-plugin`'s `errorHandler` option — so the bundler's
// exit status structurally cannot carry this, and configuring the plugin cannot change
// that. Measured 2026-09-16: the host's token had been answered `Invalid token (http
// status: 401)` on every release, and 0 of the 100 most recent `freehire-web` releases had
// any uploaded files, while every one of those deploys reported success.
//
// It checks the CREDENTIAL, not the artifact. Debug-id uploads create artifact bundles
// rather than release files, and with no successful upload anywhere in the observable
// window there is no positive control to calibrate an artifact check against — see
// design.md ("Verify the credential, not the artifact") for why a guessed field would
// reintroduce the same silence one layer up.
//
// Run from `deploy/bin/release.sh` BEFORE the web build, so a bad credential costs seconds
// rather than the three minutes the build takes.

import { fileURLToPath } from 'node:url';

/** How long to wait for Sentry before calling it unreachable. Short: this runs in front of
 *  every release, and a slow answer is treated the same as no answer either way. */
const PROBE_TIMEOUT_MS = 10_000;

/**
 * The probe reads the project's own release list. That is deliberate, not the cheapest call
 * available: it needs `project:releases`, the SAME permission the source-map upload needs,
 * so a token that can see the organisation but not write releases fails here instead of
 * sailing through and failing during the upload. It is a read, so it has no side effects.
 */
function probeUrl(baseUrl, org, project) {
  return `${baseUrl.replace(/\/+$/, '')}/api/0/projects/${org}/${project}/releases/?per_page=1`;
}

/**
 * The decision, separated from the request so it can be tested against every answer Sentry
 * can give rather than only the ones a live probe happens to produce.
 *
 * Returns `ok` (may the release continue?) and `uploading` (will this release have readable
 * traces?), because those are two different questions and the old behaviour answered
 * neither.
 *
 * @param {{configured: boolean, status?: number, unreachable?: string}} outcome
 * @returns {{ok: boolean, uploading: boolean, message: string}}
 */
export function verdict({ configured, status, unreachable }) {
  if (!configured) {
    return {
      ok: true,
      uploading: false,
      message:
        'no Sentry upload credential configured — this release will report minified stack traces',
    };
  }

  if (status !== undefined && status >= 200 && status < 300) {
    return { ok: true, uploading: true, message: 'Sentry accepted the upload credential' };
  }

  if (status === 401) {
    return {
      ok: false,
      uploading: false,
      message: 'Sentry rejected the upload credential (401) — SENTRY_AUTH_TOKEN is invalid or revoked',
    };
  }
  if (status === 403) {
    return {
      ok: false,
      uploading: false,
      message:
        'the Sentry upload credential lacks the permission the upload needs (403) — it must carry the project:releases scope',
    };
  }
  if (status === 404) {
    // Either slug is wrong, or the organisation lives in a region this base URL does not
    // serve. Both are things somebody typed, so both fail — but the message has to name the
    // second one, because it is the one nobody thinks of.
    return {
      ok: false,
      uploading: false,
      message:
        'Sentry has no such org/project (404) — check SENTRY_ORG and SENTRY_PROJECT, and SENTRY_URL if the organisation is region-pinned',
    };
  }
  // Everything left is an answer that does not decide anything: Sentry unreachable, a fault
  // of its own, or a status nobody anticipated. A rejection above is proof of a
  // misconfiguration; none of these is proof of anything, and a release path that stops on
  // them makes shipping depend on somebody else's uptime — the mistake cmd/llm-probe already
  // records, where a red signal meaning "the thing I watch is broken" must never be
  // confusable with "I am broken". So this leg passes, loudly.
  const why = unreachable ?? (status === undefined ? 'no answer' : `Sentry answered ${status}`);
  return {
    ok: true,
    uploading: true,
    message: `could not verify the Sentry upload credential (${why}) — continuing`,
  };
}

async function probe({ baseUrl, org, project, token }) {
  try {
    const res = await fetch(probeUrl(baseUrl, org, project), {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    });
    return { configured: true, status: res.status };
  } catch (err) {
    return { configured: true, unreachable: err instanceof Error ? err.message : String(err) };
  }
}

async function main() {
  const org = process.env.SENTRY_ORG?.trim();
  const project = process.env.SENTRY_PROJECT?.trim();
  const token = process.env.SENTRY_AUTH_TOKEN?.trim();
  // The same variable sentry-cli reads, so the check and the upload can never disagree about
  // which Sentry they are talking to.
  const baseUrl = process.env.SENTRY_URL?.trim() || 'https://sentry.io';

  // All three, not just the token: a release configured with two of them uploads nothing,
  // and would otherwise be indistinguishable from one configured with none.
  const configured = Boolean(org && project && token);
  const outcome = configured ? await probe({ baseUrl, org, project, token }) : { configured };

  const result = verdict(outcome);
  console.log(`[sentry-credential-check] ${result.message}`);
  process.exit(result.ok ? 0 : 1);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    // Reaching here means the check ITSELF broke, which is not evidence about the
    // credential — same rule as an unreachable Sentry, so it must not stop the release.
    console.log(`[sentry-credential-check] check failed to run (${err}) — continuing`);
    process.exit(0);
  });
}
