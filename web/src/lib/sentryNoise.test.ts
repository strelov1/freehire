import { describe, expect, it } from 'vitest';
import { isTransientNoise } from './sentryNoise';

/** The shape `$lib/api`'s `call()` throws, without importing it — see the module's own
 *  note on why the filter duck-types instead. */
const apiError = (status: number, message: string) =>
  Object.assign(new Error(message), { name: 'ApiError', status });

describe('isTransientNoise', () => {
  // Each case is a real production issue, named by the title Sentry grouped it under, so a
  // fragment removed from the filter fails against the event it was added for rather than
  // against an invented one.
  it.each([
    ['FREEHIRE-WEB-A: the SSR read timeout', apiError(504, 'The API did not answer within 10000ms: /api/v1/jobs/x')],
    ['FREEHIRE-WEB-1P: the visitor navigated away', apiError(499, 'client closed request')],
    ['FREEHIRE-WEB-1K: undici could not reach the API', new TypeError('fetch failed')],
    ['FREEHIRE-WEB-X: Safari', new TypeError('Load failed')],
    ['FREEHIRE-WEB-F: Chrome', new TypeError('Failed to fetch')],
    ['FREEHIRE-WEB-1M: Firefox', new TypeError('NetworkError when attempting to fetch resource.')],
    ['FREEHIRE-WEB-K: Firefox, shorter', new TypeError('network error')],
    ['FREEHIRE-WEB-6: a chunk the deploy deleted', new TypeError('Failed to fetch dynamically imported module: https://freehire.me/_app/immutable/nodes/0.js')],
    ['FREEHIRE-WEB-1D: the same, Firefox wording', new TypeError('error loading dynamically imported module: https://freehire.me/_app/immutable/chunks/a.js')],
    ['FREEHIRE-WEB-8: the same, Safari wording', new TypeError('Importing a module script failed.')],
    ['FREEHIRE-WEB-1R: the suggestion box abandoning a query', Object.assign(new Error('signal is aborted without reason'), { name: 'AbortError' })],
    // The abort and timeout entries below had no test, which is how a filter entry becomes
    // an assertion nobody can check. Each is a distinct MECHANISM, not a spelling: by class
    // name, by the DOMException's own default text, and by a class name flattened into the
    // message — the shape a rejected promise takes when it is re-thrown as
    // `new Error(String(err))` and loses its name on the way.
    ['an abort carrying only the DOMException default text', new Error('The operation was aborted.')],
    ['an abort whose class name survived only in its message', new Error('AbortError: the read was cancelled')],
    ['a timeout from AbortSignal.timeout(), before any ApiError wraps it', Object.assign(new Error('signal timed out'), { name: 'TimeoutError' })],
  ])('drops %s', (_, err) => {
    expect(isTransientNoise(err)).toBe(true);
  });

  // The other half of the rule, and the half worth a test: a filter that swallows a defect
  // is worse than the noise it was written for, because the quota it protects exists to
  // carry exactly these.
  it.each([
    ['a Svelte runtime crash', new Error('https://svelte.dev/e/each_key_duplicate')],
    ['a real code defect', new TypeError("Cannot read properties of undefined (reading 'slug')")],
    ['an API 500', apiError(500, 'internal server error')],
    ['an API 404', apiError(404, 'not found')],
    // A status alone proves nothing: only our own ApiError carries one that means "the
    // upstream was slow", so the same number on anything else must still be reported.
    ['a 504 on something that is not an ApiError', Object.assign(new Error('boom'), { status: 504 })],
  ])('reports %s', (_, err) => {
    expect(isTransientNoise(err)).toBe(false);
  });

  // Whatever it cannot read, it reports: silence is the failure mode this filter must not
  // introduce. FREEHIRE-WEB-29 is a rejected promise whose reason was an object with no keys.
  it.each([[null], [undefined], ['a thrown string'], [42], [{}]])(
    'reports what it cannot read: %s',
    (err) => {
      expect(isTransientNoise(err)).toBe(false);
    },
  );
});
