/** Narrows a possibly-null/undefined value, throwing if it is missing instead
 *  of silently trusting it the way a bare `!` would. */
export function must<T>(value: T | null | undefined, what = 'value'): T {
  if (value == null) throw new Error(`expected ${what} to be non-null`);
  return value;
}
