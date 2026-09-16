// Does this release's Sentry credential actually work?
//
// THIS COMMENT IS THE CANONICAL EXPLANATION. vite.config.ts, deploy/bin/release.sh,
// web/AGENTS.md and internal/platform/observability/AGENTS.md all point here rather than
// restating it: the first draft of this change spelled the argument out in five places and a
// reviewer found the mechanism misdescribed in two of them before it ever shipped. One
// measurement wants one home.
//
// It exists because nothing else can answer the question. TWO independent layers swallow a
// failed source-map upload, which is why no option set in vite.config.ts can make a bad
// credential fail the build:
//
//   1. `@sentry/bundler-plugins` calls `handleRecoverableError(e, false)` from the upload
//      hook (core/build-plugin-manager.js). With no `errorHandler` configured, `false` means
//      log and return — it does not throw, whatever the option's TypeScript doc comment says
//      about throwing by default. (That doc comment is what an earlier draft of this file
//      quoted, and it was wrong about this path.)
//   2. `@sentry/sveltekit`'s `vite/sourceMaps.js` rebuilds the upstream plugin with
//      `writeBundle: void 0` and calls the original inside a bare `catch {}` that warns and
//      returns. There is one upload — of the adapter output, covering client and SSR — and
//      this is it, so even an `errorHandler` that threw would be caught here.
//
// Measured 2026-09-16: the host's token was answered `Invalid token (http status: 401)` on
// every release, while every one of those deploys reported success.
//
// **How long that had been true was misread here, twice, and the second misreading is the
// instructive one.** An earlier version of this paragraph said "0 of the 100 most recent
// `freehire-web` releases had any uploaded file" and let that stand for "the upload has never
// worked". The count is real and it is also worthless: a release's `fileCount` is null
// whether or not the upload succeeded, because debug-id uploads create ARTIFACT BUNDLES and
// not release files. The proof arrived the day the credential was replaced — release
// `8868debd427c` uploaded 1785 files at 18:04:22Z and still reads `fileCount: null`. A
// measurement that answers the same on both sides of the thing it is measuring is not weak
// evidence, it is none.
//
// What does answer is `GET /api/0/projects/{org}/{project}/files/artifact-bundles/`. Read
// that way the real window is narrow and specific: the last bundle that landed was
// 2026-09-14T01:31:20Z, the next was 2026-09-16T18:04:22Z, and the token file on the host had
// not been touched since August — so nothing on our side changed and the credential was
// revoked or expired under us. Two days, not months.
//
// It still checks the CREDENTIAL, not the artifact, and that has not changed: an artifact
// check would have to name the field that says "this release's maps are there", and the field
// this file's own history guessed at — `fileCount` — is exactly the one that lies. See
// design.md ("Verify the credential, not the artifact"). The bundles endpoint above is for a
// HUMAN confirming an incident afterwards; wiring it in here would put a second, subtler
// guess on the release path.
//
// Run from `release.sh` BEFORE the web build, so a bad credential costs seconds rather than
// the three minutes the build takes. That script lives in the private `freehire-ops`
// repository (`scripts/host2/release.sh`) and is hand-copied to `/opt/freehire/bin` — which
// is why the "not in this checkout" branch below is an ordinary case and not a hypothetical.

import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/** How long to wait for Sentry before calling it unreachable. Short: this runs in front of
 *  every release, and a slow answer is treated the same as no answer either way. */
const PROBE_TIMEOUT_MS = 10_000;

/** The three the upload needs. Order is the order they are reported missing in. */
const REQUIRED_VARS = ['SENTRY_ORG', 'SENTRY_PROJECT', 'SENTRY_AUTH_TOKEN'];

/**
 * The status that means "I looked, and this credential will not upload" — the ONLY one a
 * caller may treat as a verdict.
 *
 * It is not 1 because 1 is what node itself exits with for an uncaught throw, a module it
 * cannot resolve, or a loader error — and what `sudo` exits with when it cannot run the
 * command at all. A caller reading 1 as a refusal would report a perfectly good token as
 * rejected the first time this file is missing from a checkout, which is exactly the
 * confusion between "the answer is no" and "there was no answer" that the whole check
 * exists to end. Above node's reserved 1-12 and below the 128+ signal range, so nothing
 * else in the chain can produce it.
 */
export const EXIT_REFUSE = 20;

function withoutTrailingSlash(url) {
  return url.replace(/\/+$/, '');
}

/** What a setting's value is, and — by returning undefined for whitespace — what counts as
 *  set at all. One definition, because `configurationOf` decides whether the credential is
 *  complete while `main` reads the same variables to probe with: two spellings of "is this
 *  set" could disagree, and the half-configured case they exist to catch is exactly where
 *  they would. */
function setting(env, name) {
  return env[name]?.trim();
}

/**
 * The two reads, in the order they answer distinct questions.
 *
 * `chunk-upload` is what `sentry-cli` itself fetches first, to negotiate chunk options before
 * uploading anything — so probing it asks the closest question to the upload's own that a
 * side-effect-free read can ask. The earlier draft probed the project's release LIST, which
 * Sentry also grants to a read-only `project:read` token, so an under-scoped credential would
 * have passed the check and failed the upload — the exact case the check exists to catch.
 *
 * **It is not yet proven that this endpoint rejects an under-scoped token.** Sentry's scope
 * map for it may admit `org:read` beside the write scope, since the upload is the `POST` and
 * this is the `GET`; nobody has tried one. Until a deliberately `project:read`-only token has
 * been shown to fail here (task 5.4), "a token the upload cannot use cannot get through" is
 * the intent of this probe, not a demonstrated property of it. The 403 branch below, and the
 * `project:releases` it names, rest on the same unproven reading.
 *
 * The project read stays as a second step because `chunk-upload` is organisation-scoped and so
 * cannot see a mistyped `SENTRY_PROJECT` — which would upload into nothing, silently, in the
 * same way.
 */
export function probeUrls(baseUrl, org, project) {
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
  const missing = REQUIRED_VARS.filter((name) => !setting(env, name));
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
  const org = setting(process.env, 'SENTRY_ORG');
  const project = setting(process.env, 'SENTRY_PROJECT');
  const token = setting(process.env, 'SENTRY_AUTH_TOKEN');
  // Read by `@sentry/bundler-plugins` (options-mapping.js: `userOptions.url ??
  // process.env["SENTRY_URL"] ?? SENTRY_SAAS_URL`), which hands it to sentry-cli — so as long
  // as release.sh passes it through to the build too, the check and the upload cannot disagree
  // about which Sentry they mean.
  const baseUrl = setting(process.env, 'SENTRY_URL') || 'https://sentry.io';

  const { configuration, missing } = configurationOf(process.env);
  const outcome =
    configuration === 'complete'
      ? { configuration, org, project, ...(await probe({ baseUrl, org, project }, token)) }
      : { configuration, missing };

  const result = verdict(outcome);
  console.log(`[sentry-credential-check] ${result.message}`);
  process.exit(result.ok ? 0 : EXIT_REFUSE);
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
