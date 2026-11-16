// What the chat does by itself: the turn a fresh conversation starts with, and how the pane
// follows the address its host shows. Pure decisions; AssistantChat carries them out.

import type { TurnStart } from '#lib/assistant/client.js';
import type { ChatPreset } from '#lib/assistant/presets.js';

/**
 * What the URL the chat mounted on asked for: which conversation to mint, and the opening
 * message to send into it. Read once, and each half spent once.
 *
 * Both are properties of the ARRIVAL, not of the surface. The address is rewritten to the
 * session's own URL moments later — reading them reactively would race that rewrite — and
 * the chat outlives the rewrite now that one route node serves every chat. Left standing,
 * they would mint a second interview on the next `boot()` and put the same words in the
 * caller's mouth again.
 */
export type Arrival = { preset: ChatPreset; kickoff?: string };

/**
 * The turn an empty conversation starts by itself, or null.
 *
 * Autostart: the host can pass a kickoff prompt so the agent begins immediately instead of
 * waiting for the user to type. Only on an empty session, and only once — boot() runs again
 * whenever the caller returns to the bare address, and putting the same words in their
 * mouth a second time is not what they asked for (the caller spends the kickoff).
 *
 * A rehearsal speaks first. There is no kickoff text to carry because the brief is the
 * server's — the candidate came here from an application, and the agent opens by reading
 * its context. The empty-transcript guard is the client half of the backend's: a reload
 * replays the conversation rather than restarting it.
 *
 * Cold start: the tailoring workspace runs the unattended pass itself instead of offering
 * the two-action menu. Same empty-transcript guard — a session that already has a message
 * (the run already started, or the candidate answered its closing question) must not be
 * run again.
 */
export function autostartTurn(o: {
  kickoff: string | undefined;
  empty: boolean;
  preset: string | null;
  autoRun: boolean;
}): TurnStart | null {
  if (!o.empty) return null;
  if (o.kickoff) return { kind: 'message', text: o.kickoff };
  if (o.preset === 'interview') return { kind: 'opening' };
  if (o.autoRun) return { kind: 'autopilot' };
  return null;
}

/**
 * What following the address `requested` takes.
 *
 * A switch the chat started asked the host to navigate, and that navigation is async: for
 * the moment between setting activeId and the URL catching up, the address disagrees with
 * activeId and following it would go straight back to the chat just left. That is what
 * evicted a newly created chat. The guard (`navigatingTo`) clears the moment the host lands
 * — BEFORE any early return, or the guard outlives its navigation and the pane stops
 * following the URL at all, which is Back and Forward silently doing nothing.
 *
 * The bare address. Since one route node serves both it and a chat's own URL, getting here
 * is a prop change rather than a fresh mount: nothing re-runs on its own, so a dead link's
 * "Open your chats" and the nav rail's own entry would both leave the caller staring at
 * whatever was already on screen. Boot again, which is what a remount used to do — open the
 * newest chat and re-address to it. A blank chat lives at the bare address (it has no id to
 * put there): booting again would land right back on it and wake the follower forever.
 */
export function addressStep(
  requested: string | undefined,
  pane: { navigatingTo: string | null; deferredNew: boolean; activeId: string | null },
): { clearGuard: boolean; action: 'none' | 'boot' | 'open' } {
  const clearGuard = !!pane.navigatingTo && requested === pane.navigatingTo;
  if (pane.navigatingTo && !clearGuard) return { clearGuard, action: 'none' };
  if (!requested) {
    return { clearGuard, action: pane.deferredNew && !pane.activeId ? 'none' : 'boot' };
  }
  return { clearGuard, action: requested === pane.activeId ? 'none' : 'open' };
}
