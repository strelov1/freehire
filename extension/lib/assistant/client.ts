// The panel's half of one assistant turn, ported from the web app's client
// (`web/src/lib/assistant/client.ts` in the hire repo).
//
// A turn is a single POST whose response body streams the turn as SSE. That is the
// whole transport: no connection held open between turns, no attach, no input
// lease. Cancelling is aborting the fetch — the backend notices its next write fail
// and stops the loop before spending another model call.
//
// Divergence from the web's copy: an absolute origin and a Bearer credential, since
// extension code cannot see hire's httpOnly cookie across origins.

import { HIRE_ORIGIN } from '../auth';
import { readFrames } from './sse';
import type { TurnEvent } from './wire';

const BASE = `${HIRE_ORIGIN}/api/v1/assistant`;

/** A turn in flight: its completion, and the handle that stops it. */
export interface Turn {
  done: Promise<void>;
  cancel: () => void;
}

/**
 * Send a message and stream the turn. `onEvent` receives every frame in order,
 * ending with exactly one `result`. The returned promise resolves when the stream
 * ends — including when it was cancelled, which is a normal outcome rather than an
 * error the user must act on.
 */
export function sendTurn(
  sessionId: string,
  text: string,
  token: string,
  onEvent: (e: TurnEvent) => void,
): Turn {
  const controller = new AbortController();

  const done = (async () => {
    // Divergence from the web's copy: the fetch is inside the abort guard, not
    // outside it. Stop renders the moment a turn starts, so it is reachable before
    // the response headers arrive — and there the abort rejects the fetch itself,
    // which the web surfaces as a raw AbortError on screen.
    let res: Response;
    try {
      res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/messages`, {
        method: 'POST',
        credentials: 'omit',
        headers: {
          'content-type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ text }),
        signal: controller.signal,
      });
    } catch (e) {
      if (controller.signal.aborted) {
        onEvent({ type: 'result', stop_reason: 'cancelled' });
        return;
      }
      throw e;
    }
    if (!res.ok) {
      throw new Error(`could not send the message (${res.status})`);
    }
    if (!res.body) {
      throw new Error('the assistant returned no stream');
    }
    try {
      await readFrames(res.body, (frame) => {
        const event = decodeEvent(frame.data);
        if (event) onEvent(event);
      });
    } catch (e) {
      // An aborted read is the cancellation we asked for, not a failure. The turn
      // still has to end with a terminal event, or the composer waits forever for
      // one the backend will never send.
      if (controller.signal.aborted) {
        onEvent({ type: 'result', stop_reason: 'cancelled' });
        return;
      }
      throw e;
    }
  })();

  return { done, cancel: () => controller.abort() };
}

/** Decode one frame's payload. A frame we cannot parse is dropped rather than
 *  thrown: one malformed frame must not abandon a turn that is otherwise fine. */
function decodeEvent(data: string): TurnEvent | null {
  try {
    return JSON.parse(data) as TurnEvent;
  } catch (e) {
    console.error('assistant: invalid event payload', e, data);
    return null;
  }
}
