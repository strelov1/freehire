/**
 * Which conversation the panel is holding, remembered across openings — and which
 * page it was about, so a conversation started on one job posting is never resumed
 * on another. Only the id and the page key are kept: the transcript lives on the
 * server, where the web app can continue the same conversation — caching messages
 * here would give one exchange two sources of truth. Shaped like `auth.ts`'s token
 * helpers, and thin for the same reason: it is storage plumbing, not logic.
 */

import { browser } from 'wxt/browser';

const SESSION_KEY = 'assistantSession';

export interface StoredSession {
  id: string;
  /** The page key (see App.svelte's `pageKey`) the conversation was started on. */
  pageKey: string;
}

/** The conversation to resume, or null if the panel is starting fresh. */
export async function recallSession(): Promise<StoredSession | null> {
  const stored = await browser.storage.local.get(SESSION_KEY);
  const raw = stored[SESSION_KEY];
  return raw && typeof raw === 'object' ? (raw as StoredSession) : null;
}

/** Remember the conversation the panel is now holding, and the page it is about. */
export async function rememberSession(id: string, pageKey: string): Promise<void> {
  await browser.storage.local.set({ [SESSION_KEY]: { id, pageKey } satisfies StoredSession });
}

/** Forget it — on sign-out, on reset, when the page changes, and when the server no
 *  longer has it. */
export async function forgetSession(): Promise<void> {
  await browser.storage.local.remove(SESSION_KEY);
}
