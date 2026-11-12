// The panel's half of one assistant turn, ported from the web app's client
// (`web/src/lib/assistant/client.ts` in the hire repo).
//
// A turn is a single POST whose response body streams the turn as SSE. That is the
// whole transport: no connection held open between turns, no attach, no input
// lease. Cancelling explicitly asks the server to stop the turn, then aborts the
// local reader: a reader disconnect alone deliberately leaves the turn running.
//
// Divergence from the web's copy: an absolute origin and a Bearer credential, since
// extension code cannot see hire's httpOnly cookie across origins.

import { HIRE_ORIGIN } from '../auth';
import { PlanRefused, planRefusedFrom } from '../planRefused';
import { readFrames } from './sse';
import type { TurnEvent } from './wire';

const BASE = `${HIRE_ORIGIN}/api/v1/assistant`;

/** A turn in flight: its completion, and the handle that stops it. */
export interface Turn {
  done: Promise<void>;
  cancel: () => void;
}

/** Cancellation is independent of the reader: its signal must not abort this request. */
async function cancelTurn(sessionId: string, token: string): Promise<void> {
  try {
    await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/cancel`, {
      method: 'POST',
      credentials: 'omit',
      headers: { Authorization: `Bearer ${token}` },
    });
  } catch {
    // Best-effort while offline. The local reader still stops, and the server's
    // own step and timeout bounds limit work if cancellation cannot reach it.
  }
}

/**
 * Send a message and stream the turn. `onEvent` receives every frame in order,
 * ending with exactly one `result`. The returned promise resolves when the stream
 * ends — including when it was cancelled, which is a normal outcome rather than an
 * error the user must act on.
 */
export function sendTurn(sessionId: string, text: string, token: string, onEvent: (e: TurnEvent) => void): Turn {
  const controller = new AbortController();
  let cancellationRequested = false;
  let accepted = false;

  const stopAcceptedTurn = () => {
    void cancelTurn(sessionId, token);
    controller.abort();
  };

  const done = (async () => {
    const res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/messages`, {
      method: 'POST',
      credentials: 'omit',
      headers: {
        'content-type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ text }),
      signal: controller.signal,
    });
    if (res.status === 402) {
      throw await planRefusal(res);
    }
    if (!res.ok) {
      throw new Error(`could not send the message (${res.status})`);
    }
    // Successful headers prove the server has claimed the turn. An earlier
    // cancel request could arrive before that claim and do nothing; keep the
    // message request alive until this point even if Stop was already pressed.
    accepted = true;
    if (cancellationRequested) {
      stopAcceptedTurn();
      onEvent({ type: 'result', stop_reason: 'cancelled' });
      return;
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

  return {
    done,
    cancel: () => {
      cancellationRequested = true;
      if (accepted && !controller.signal.aborted) stopAcceptedTurn();
    },
  };
}

/** Read a 402's body into a PlanRefused. An unreadable body must still surface as
 *  the refusal it is, not as a second, unrelated failure. */
async function planRefusal(res: Response): Promise<PlanRefused> {
  const fallback = 'Your plan does not include the assistant.';
  try {
    return planRefusedFrom(await res.json(), fallback);
  } catch {
    return new PlanRefused(fallback, null);
  }
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
