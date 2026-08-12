/** Exhaustiveness check for a `switch`/`if` over a closed union: put this in the
 *  `default`/final branch so an unhandled member fails to compile instead of
 *  silently falling through. Do not use where the union is meant to stay open
 *  (e.g. forward-compat with server-sent values) — a thrown error there is the
 *  wrong failure mode. */
export function assertNever(x: never): never {
  throw new Error(`Unreachable case: ${JSON.stringify(x)}`);
}
