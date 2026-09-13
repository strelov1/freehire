// Persists a candidate's "I'm not dealing with this attempt right now" marker for one
// auto-apply queue entry, purely as a personal reminder in the tracker drawer's Progress
// tab. It has no effect on cmd/auto-apply's claim/process behavior or on the queue row —
// see openspec/changes/auto-apply-progress-tab/design.md.
//
// Self-contained on purpose, mirroring filterStorage.ts: it feature-detects `localStorage`
// rather than importing `browser` from `$app/environment`, so the unit test runs in the
// plain-Node vitest env. Every access is wrapped and failures are swallowed — private mode
// / quota / disabled storage must never break the drawer, which still works without it.

const KEY_PREFIX = 'hire.autoApplyPaused:';

/** Whether the given queue entry is marked paused. Defaults to false when storage is
 *  unavailable or the entry was never marked. */
export function isAutoApplyPaused(queueId: number): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    return localStorage.getItem(KEY_PREFIX + queueId) !== null;
  } catch {
    return false;
  }
}

/** Mirror the paused marker to storage. Unpausing removes the key, so a queue id that was
 *  never paused and one that was paused and then resumed are indistinguishable — the only
 *  state this needs to hold. */
export function setAutoApplyPaused(queueId: number, paused: boolean): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (paused) localStorage.setItem(KEY_PREFIX + queueId, '1');
    else localStorage.removeItem(KEY_PREFIX + queueId);
  } catch {
    // best-effort: private mode / quota / disabled storage
  }
}
