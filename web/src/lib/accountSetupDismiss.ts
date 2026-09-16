// Per-account dismissal of the setup card's "alerts" step, kept free of Svelte runes and
// SvelteKit imports so it unit-tests in the plain-node vitest environment, mirroring
// supportToast.ts.
//
// Keyed by account id, not a single flag: the setup card renders on a per-user basis, and
// a shared computer must not carry one account's "I'm not job-hunting right now" answer
// into the next person's session.

function key(userId: number): string {
  return `hire.account-setup-alerts-dismissed:${userId}`;
}

/** Whether this account dismissed the alerts step. False for a signed-out reader (no
 *  account id yet) and for unavailable storage (Safari private mode, a blocked origin) —
 *  showing the step to someone who dismissed it is a smaller harm than a throw in the
 *  layout. */
export function readAlertsDismissed(userId: number | undefined): boolean {
  if (userId === undefined) return false;
  try {
    return localStorage.getItem(key(userId)) === '1';
  } catch {
    return false;
  }
}

/** Record that this account dismissed the step. Silently a no-op when there is no account
 *  id yet or storage is unavailable; the dismissal then lasts for this page only. */
export function writeAlertsDismissed(userId: number | undefined): void {
  if (userId === undefined) return;
  try {
    localStorage.setItem(key(userId), '1');
  } catch {
    /* storage unavailable; the dismissal lasts for this page only */
  }
}
