// Keep transport/cancellation noise out of exception reporting, matching the web policy.
const TRANSIENT_API_STATUS = new Set([499, 504]);

const TRANSIENT_MESSAGES = [
  'failed to fetch',
  'load failed',
  'network error',
  'networkerror',
  'fetch failed',
  'dynamically imported module',
  'importing a module script failed',
  'signal is aborted without reason',
  'the operation was aborted',
  'aborterror',
] as const;

const TRANSIENT_NAMES = new Set(['AbortError', 'TimeoutError']);

export function isTransientNoise(err: unknown): boolean {
  if (err == null || typeof err !== 'object') return false;
  const e = err as { name?: unknown; message?: unknown; status?: unknown };

  if (typeof e.name === 'string' && TRANSIENT_NAMES.has(e.name)) return true;

  if (e.name === 'ApiError' && typeof e.status === 'number' && TRANSIENT_API_STATUS.has(e.status)) {
    return true;
  }

  if (typeof e.message !== 'string') return false;
  const message = e.message.toLowerCase();
  return TRANSIENT_MESSAGES.some((fragment) => message.includes(fragment));
}
