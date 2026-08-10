/**
 * Which conversation the panel is holding, remembered across openings.
 *
 * Only the id is kept. The transcript lives on the server, where the web app can
 * continue the same conversation — caching messages here would give one exchange
 * two sources of truth. Shaped like `auth.ts`'s token helpers, and thin for the
 * same reason: it is storage plumbing, not logic.
 */

import { browser } from 'wxt/browser';

const SESSION_KEY = 'assistantSessionId';

/** The conversation to resume, or null if the panel is starting fresh. */
export async function recallSession(): Promise<string | null> {
  const stored = await browser.storage.local.get(SESSION_KEY);
  return (stored[SESSION_KEY] as string) ?? null;
}

/** Remember the conversation the panel is now holding. */
export async function rememberSession(id: string): Promise<void> {
  await browser.storage.local.set({ [SESSION_KEY]: id });
}

/** Forget it — on sign-out, on "new chat", and when the server no longer has it. */
export async function forgetSession(): Promise<void> {
  await browser.storage.local.remove(SESSION_KEY);
}
