// Follow-up suggestions: when to ask for them, and how they are shown.
//
// The suggestions themselves come from the server, which caps how many and how long
// they may be. What is here is the client's half: the decision to spend the call at
// all, and the display shaping — which never turns a suggestion into markup, because
// clicking one speaks in the caller's own voice and what they agree to has to be what
// they read.

import type { TurnEvent } from './wire';

/** How much of a suggestion is shown before it is elided.
 *
 *  Deliberately the same number as the server's own per-item cap, which DISCARDS
 *  anything longer. That makes the elision unreachable in practice, and that is the
 *  point: what is sent is what was displayed, so a shorter cap here would mean the
 *  caller agreeing to one question and sending a different one. This stays as the
 *  defence for the day the server's cap moves and this file does not. */
export const MAX_DISPLAY_LEN = 120;

/** The `result` event a settled turn ends with. */
type Result = Extract<TurnEvent, { type: 'result' }>;

/** Whether a turn that just ended is worth suggesting from.
 *
 *  Only a turn that ran to completion AND said something. A turn that errored, was
 *  cancelled, or hit the step ceiling leaves the conversation in a state the caller
 *  has to resolve, and offering them a next question there reads as if nothing had
 *  gone wrong. An unrecognised stop reason is treated the same way: the backend owns
 *  that vocabulary, and a value this client does not know is a reason to stay quiet
 *  rather than to guess. */
export function shouldRequest(result: Result, answer: string): boolean {
  if (result.is_error) return false;
  if (result.stop_reason !== 'end_turn') return false;
  return answer.trim() !== '';
}

/** One suggestion as it is shown.
 *
 *  Whitespace is collapsed because a suggestion is rendered on one row and a newline
 *  the model wrote would break it. Nothing here escapes anything: the caller renders
 *  this as a text node, never as markup, so there is no markup to escape. */
export function forDisplay(suggestion: string): string {
  const flat = suggestion.replace(/\s+/g, ' ').trim();
  return flat.length > MAX_DISPLAY_LEN ? `${flat.slice(0, MAX_DISPLAY_LEN)}…` : flat;
}
