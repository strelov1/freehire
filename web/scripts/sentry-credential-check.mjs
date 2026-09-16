// Does this release's Sentry credential actually work?
//
// It exists because nothing else can answer that. `@sentry/sveltekit`'s
// `vite/sourceMaps.js` builds its own copy of the upstream plugin with `writeBundle: void 0`
// and calls the original inside a bare `catch {}` that warns and returns — so that catch is
// the ONLY upload path, for both the client and the SSR build, and it sits above
// `@sentry/vite-plugin`'s `errorHandler` option whose documented default is to throw and stop
// the bundle. No option set in vite.config.ts can make a bad credential fail the build.
// Measured 2026-09-16: the host's token had been answered `Invalid token (http status: 401)`
// on every release, and 0 of the 100 most recent `freehire-web` releases had any uploaded
// file, while every one of those deploys reported success.
//
// It checks the CREDENTIAL, not the artifact. Debug-id uploads create artifact bundles
// rather than release files, and with no successful upload anywhere in the observable window
// there is no positive control to calibrate an artifact check against — see design.md
// ("Verify the credential, not the artifact") for why a guessed field would reintroduce the
// same silence one layer up.
//
// Run from `deploy/bin/release.sh` BEFORE the web build, so a bad credential costs seconds
// rather than the three minutes the build takes.

import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/** How long to wait for Sentry before calling it unreachable. Short: this runs in front of
 *  every release, and a slow answer is treated the same as no answer either way. */
const PROBE_TIMEOUT_MS = 10_000;

/** The three the upload needs. Order is the order they are reported missing in. */
const REQUIRED_VARS = ['SENTRY_ORG', 'SENTRY_PROJECT', 'SENTRY_AUTH_TOKEN'];

function withoutTrailingSlash(url) {
  return url.replace(/\/+$/, '');
}

/**
 * The two reads, in the order they answer distinct questions.
 *
 * `chunk-upload` is what `sentry-cli` itself fetches first, to negotiate chunk options before
 * uploading anything. Probing it is what makes "the check needs what the upload needs" true by
 * construction rather than by reading a permission table: a token the upload cannot use cannot
 * get through here either. The earlier draft probed the project's release LIST, which Sentry
 * also grants to a read-only `project:read` token — so an under-scoped credential would have
 * passed the check and failed the upload, which is the exact case the check exists to catch.
 *
 * The project read stays as a second step because `chunk-upload` is organisation-scoped and so
 * cannot see a mistyped `SENTRY_PROJECT` — which would upload into nothing, silently, in the
 * same way.
 */
function probeUrls(baseUrl, org, project) {
  const root = withoutTrailingSlash(baseUrl);
  return [
    `${root}/api/0/organizations/${org}/chunk-upload/`,
    `${root}/api/0/projects/${org}/${project}/releases/?per_page=1`,
  ];
}

/**
 * The decision, separated from the requests so it can be tested against every answer Sentry
 * can give rather than only the ones a live probe happens to produce.
 *
 * @param {{configuration: 'none'|'partial'|'complete', missing?: string[], status?: number,
 *          unreachable?: string, org?: string, project?: string}} outcome
 * @returns {{ok: boolean, message: string}}
 */
export function verdict({ configuration, missing = [], status, unreachable, org, project }) {
  if (configuration === 'none') {
    return {
      ok: true,
      message:
        'no Sentry upload credential configured — this release will report minified stack traces',
    };
  }

  // Some but not all. Uploads nothing, exactly like an opt-out, which is why it has to be
  // called out rather than folded into the branch above: a rotation that writes a truncated
  // env file would otherwise ship green claiming it meant to.
  if (configuration === 'partial') {
    return {
      ok: false,
      message: `the Sentry upload credential is only partly configured — missing ${missing.join(', ')}`,
    };
  }

  if (status !== undefined && status >= 200 && status < 300) {
    return { ok: true, message: `Sentry accepted the upload credential (${org}/${project})` };
  }

  if (status === 401) {
    return {
      ok: false,
      message: 'Sentry rejected the upload credential (401) — SENTRY_AUTH_TOKEN is invalid or revoked',
    };
  }
  if (status === 403) {
    return {
      ok: false,
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
  return { ok: true, message: `could not verify the Sentry upload credential (${why}) — continuing` };
}

/** Which of the required variables are set, and which are not. Exported for its own test:
 *  it decides WHICH QUESTION gets asked, so getting it wrong is not visible in verdict(). */
export function configurationOf(env) {
  const missing = REQUIRED_VARS.filter((name) => !env[name]?.trim());
  if (missing.length === 0) return { configuration: 'complete', missing };
  if (missing.length === REQUIRED_VARS.length) return { configuration: 'none', missing };
  return { configuration: 'partial', missing };
}

async function probe({ baseUrl, org, project }, token) {
  // First failure wins: once one read has said the credential is wrong, the second cannot
  // add anything, and asking anyway would report the later answer for the earlier fault.
  let outcome;
  for (const url of probeUrls(baseUrl, org, project)) {
    try {
      const res = await fetch(url, {
        headers: { Authorization: `Bearer ${token}` },
        signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
      });
      outcome = { status: res.status };
      if (res.status < 200 || res.status >= 300) return outcome;
    } catch (err) {
      return { unreachable: err instanceof Error ? err.message : String(err) };
    }
  }
  return outcome;
}

async function main() {
  const org = process.env.SENTRY_ORG?.trim();
  const project = process.env.SENTRY_PROJECT?.trim();
  const token = process.env.SENTRY_AUTH_TOKEN?.trim();
  // Read by `@sentry/bundler-plugins` (options-mapping.js: `userOptions.url ??
  // process.env["SENTRY_URL"] ?? SENTRY_SAAS_URL`), which hands it to sentry-cli — so as long
  // as release.sh passes it through to the build too, the check and the upload cannot disagree
  // about which Sentry they mean.
  const baseUrl = process.env.SENTRY_URL?.trim() || 'https://sentry.io';

  const { configuration, missing } = configurationOf(process.env);
  const outcome =
    configuration === 'complete'
      ? { configuration, org, project, ...(await probe({ baseUrl, org, project }, token)) }
      : { configuration, missing };

  const result = verdict(outcome);
  console.log(`[sentry-credential-check] ${result.message}`);
  process.exit(result.ok ? 0 : 1);
}

// realpath on both sides: `import.meta.url` is resolved by the ESM loader and `process.argv[1]`
// is not, so invoking this through a symlink would compare two different strings, run nothing,
// and exit 0 — a silence-ending check that silently passes, which is the worst outcome
// available to it. This host reaches most of its code through the `hire-current` symlink.
function invokedDirectly() {
  try {
    return realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url));
  } catch {
    return false;
  }
}

if (invokedDirectly()) {
  main().catch((err) => {
    // Reaching here means the check ITSELF broke, which is not evidence about the
    // credential — same rule as an unreachable Sentry, so it must not stop the release.
    console.log(`[sentry-credential-check] check failed to run (${err}) — continuing`);
    process.exit(0);
  });
}
