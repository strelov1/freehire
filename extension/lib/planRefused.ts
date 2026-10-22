/**
 * The plan refused the call (HTTP 402). The server writes the sentence for the
 * candidate — what their plan lacks or what ran out — and, when there is a bigger
 * plan to buy, the path to upgrade at; both are carried so the panel can show a
 * paywall rather than a status code.
 *
 * Shared by every surface that makes a model call — the assistant's streamed turn
 * and the plain JSON calls in `freehire.ts` (agent autofill) — because the 402 body
 * is one shape server-side, and a surface that read it differently is how autofill
 * ended up printing "(HTTP 402)" with no way to upgrade.
 */
export class PlanRefused extends Error {
  /** A path on the hire origin (e.g. `/my/plan`), or null when nothing is for sale. */
  readonly upgradeUrl: string | null;
  /** The metered feature that refused (e.g. `assistant`), or `unknown`. */
  readonly feature: string;

  constructor(message: string, upgradeUrl: string | null, feature = 'unknown') {
    super(message);
    this.name = 'PlanRefused';
    this.upgradeUrl = upgradeUrl;
    this.feature = feature;
  }
}

/** Reads a parsed 402 body. Any field that is missing or mistyped falls back rather
 *  than throwing: an unreadable refusal must still surface as the refusal it is. */
export function planRefusedFrom(body: unknown, fallback: string): PlanRefused {
  const b = (typeof body === 'object' && body !== null ? body : {}) as {
    error?: unknown;
    upgrade_url?: unknown;
    allowance?: { feature?: unknown };
  };
  return new PlanRefused(
    typeof b.error === 'string' ? b.error : fallback,
    typeof b.upgrade_url === 'string' ? b.upgrade_url : null,
    typeof b.allowance?.feature === 'string' ? b.allowance.feature : 'unknown',
  );
}
