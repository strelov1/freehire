import type { PrefillResult } from './types';

/** Whether a link prefill carried anything worth filling in. A skills list that came back
 *  empty is truthy but empty, so arrays are measured rather than tested. */
export function prefillFound(result: PrefillResult): boolean {
  return Object.values(result).some((v) =>
    Array.isArray(v) ? v.length > 0 : typeof v === 'string' && v.trim() !== '',
  );
}

/** A link prefill may only fill a field the user has not typed into: a late-arriving
 *  result must never clobber manual entry. `convert` maps the incoming value (the source
 *  page's HTML description to markdown). */
export function fillEmpty(current: string, incoming?: string, convert?: (v: string) => string): string {
  if (current.trim() !== '' || !incoming) return current;
  return convert ? convert(incoming) : incoming;
}
