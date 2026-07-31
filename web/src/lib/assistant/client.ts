// The browser half of one assistant turn.
//
// A turn is a single POST whose response body streams the turn as SSE. That is
// the whole client: no connection held open between turns, no attach, no input
// lease. Cancelling is aborting the fetch — the backend notices its next write
// fail and stops the loop before spending another model call.

import { readFrames } from './sse';
import type { TurnEvent } from './wire';

const BASE = '/api/v1/assistant';

/** A turn in flight: its completion, and the handle that stops it. */
export interface Turn {
  done: Promise<void>;
  cancel: () => void;
}

/**
 * Send a message and stream the turn. `onEvent` receives every frame in order,
 * ending with exactly one `result`. The returned promise resolves when the stream
 * ends — including when it was cancelled, which is a normal outcome rather than
 * an error the user must act on.
 */
export function sendTurn(sessionId: string, text: string, onEvent: (e: TurnEvent) => void): Turn {
  return streamTurn(
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/messages`,
    { text },
    'could not send the message',
    onEvent,
  );
}

/**
 * Start an unattended tailoring run on a tailoring conversation. It streams exactly like a
 * message — same frames, same cancellation — but carries no text: the brief and the turn's
 * ceiling are the server's, so there is nothing for a client to compose or to raise.
 */
export function startAutopilot(sessionId: string, onEvent: (e: TurnEvent) => void): Turn {
  return streamTurn(
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/autopilot`,
    {},
    'could not start the run',
    onEvent,
  );
}

/**
 * Speak first in a rehearsal. The candidate opened it from an application and has nothing
 * to type, so the opening turn carries a brief the server chose — this call has no text for
 * the same reason the autopilot's does not.
 *
 * The backend refuses a rehearsal that already has a transcript, so a reload replays the
 * conversation instead of restarting the interview.
 */
export function openRehearsal(sessionId: string, onEvent: (e: TurnEvent) => void): Turn {
  return streamTurn(
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/opening`,
    {},
    'could not open the rehearsal',
    onEvent,
  );
}

/** POST a turn and stream its frames. Shared by every way of starting one, so cancellation
 *  and frame decoding have a single implementation. */
function streamTurn(
  url: string,
  body: unknown,
  failure: string,
  onEvent: (e: TurnEvent) => void,
): Turn {
  const controller = new AbortController();

  const done = (async () => {
    const res = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    if (!res.ok) {
      throw new Error(`${failure} (${res.status})`);
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
      // An aborted read is the cancellation we asked for, not a failure.
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
