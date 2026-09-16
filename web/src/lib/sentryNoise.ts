// What must never reach Sentry, because reporting it costs a real error its place.
//
// The quota is the design constraint, not a nicety. This deployment runs Sentry's free
// tier — 5 000 errors a month for the whole organisation — and both the browser and the
// SSR process report into the same project. Measured over 2026-08-29..09-16, 5 044 events
// were accepted and a further 2 419 were rejected with `error_usage_exceeded`: the month's
// allowance ran out on the 14th and every fault after it, including a production outage on
// the 15th, was thrown away at the door. An empty issue list then reads as "nothing broke",
// which is the one thing an error tracker must never say when it is simply switched off.
//
// ~70% of what was accepted was a single shape: an upstream request that did not answer in
// time, one event per visitor per page, arriving in the thousands exactly when the site is
// already known to be struggling. None of those events is actionable on its own — they
// carry no stack we can act on, only the fact that the backend was slow, which the host's
// own latency metrics already say more precisely and without a per-request cost.
//
// The rule this module encodes: report a DEFECT, not a CONDITION. A condition is
// transport, timing, cancellation, or a browser holding markup a deploy has replaced —
// all of them observable elsewhere (nginx, the uptime check, the release log) and none of
// them fixed by reading a thousand copies. A defect is anything that would still be wrong
// if the network were perfect.
//
// Self-contained on purpose, for the reason themeStorage.ts records for itself: it
// duck-types the error rather than importing ApiError from $lib/api, so the unit test runs
// in the plain-Node vitest env and the filter cannot drag the API client into either hook's
// startup path.

/** Our own SSR read timeout, and the status the API answers with when the visitor
 *  navigated away mid-request. Both are named in `internal/api/handler/errors.go`
 *  (`statusClientClosedRequest`) and raised in `$lib/api`'s `call()`. */
const TRANSIENT_API_STATUS = new Set([499, 504]);

/** Message fragments, matched lowercased. Each line is one browser's or runtime's wording
 *  for the same condition — the variety is why this is a list and not a predicate. */
const TRANSIENT_MESSAGES = [
  // Transport. Chrome says "Failed to fetch", Safari "Load failed", Firefox
  // "NetworkError when attempting to fetch resource", undici (the SSR fetch) "fetch
  // failed". A systemic version of this is an outage, and an outage is not discovered by
  // reading Sentry.
  'failed to fetch',
  'load failed',
  'network error',
  'networkerror',
  'fetch failed',
  // A tab open across a deploy still names `_app/immutable` chunks the release has
  // deleted. `+layout.svelte` already turns the next navigation into a full page load once
  // the version poll notices (see the `updated.current` guard there); what reaches here is
  // the residue that raced the poll, and no further code change can catch it.
  'dynamically imported module',
  'importing a module script failed',
  // Cancellation. The suggestion box aborts its own in-flight requests on every settled
  // keystroke by design, and a visitor leaving a page aborts the rest.
  'signal is aborted without reason',
  'the operation was aborted',
  'aborterror',
] as const;

/** Error class names that are a condition whatever they carry. */
const TRANSIENT_NAMES = new Set(['AbortError', 'TimeoutError']);

/**
 * True when this failure is a transient condition rather than a defect, and so must be
 * dropped before it is billed against the month's allowance.
 *
 * Takes `unknown` because it is called with whatever was thrown: a real Error, our
 * ApiError, a DOMException, or — for a rejected promise with no reason — something with
 * no shape at all. Anything it cannot read is reported, because the failure mode to avoid
 * is silence, not noise.
 */
export function isTransientNoise(err: unknown): boolean {
  if (err == null || typeof err !== 'object') return false;
  const e = err as { name?: unknown; message?: unknown; status?: unknown };

  if (typeof e.name === 'string' && TRANSIENT_NAMES.has(e.name)) return true;

  // `status` is only consulted for our own ApiError: a bare 504 on some other object is
  // not evidence of anything, and reading it would be a filter keyed on a field name
  // rather than on what raised it.
  if (e.name === 'ApiError' && typeof e.status === 'number' && TRANSIENT_API_STATUS.has(e.status)) {
    return true;
  }

  if (typeof e.message !== 'string') return false;
  const message = e.message.toLowerCase();
  return TRANSIENT_MESSAGES.some((fragment) => message.includes(fragment));
}
