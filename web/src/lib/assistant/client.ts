// The browser half of one assistant turn.
//
// A turn is a single POST whose response body streams the turn as SSE. That is
// the whole client: no connection held open between turns, no attach, no input
// lease.
//
// Reading and stopping are now two different things. Aborting the fetch stops THIS
// client reading; it does not stop the turn, which runs on the server under its own
// bounds and stores its transcript whether anyone is listening or not. Stopping the
// turn is a request of its own — which is what lets a phone background its tab
// without throwing the work away.

import { readFrames } from './sse';
import type { TurnEvent } from './wire';

const BASE = '/api/v1/assistant';

/** A turn in flight: its completion, and the handle that stops it. */
export interface Turn {
  done: Promise<void>;
  cancel: () => void;
}

/**
 * The stream ended before the turn did — a dropped connection, a frozen tab, a slept
 * laptop.
 *
 * It is deliberately its own type because it is NOT a failed turn: the turn is still
 * running on the server and its transcript is stored, so the caller should re-read the
 * session rather than tell the user their work failed. Saying "error" here would
 * misreport the state of the user's own CV.
 */
export class StreamInterrupted extends Error {
  constructor(cause: unknown) {
    super('the stream was interrupted', { cause });
    this.name = 'StreamInterrupted';
  }
}

/** Ask the server to stop a session's running turn. Safe to call when nothing is
 *  running: the caller cannot know whether the turn it stopped watching has ended. */
async function cancelTurn(sessionId: string): Promise<void> {
  try {
    await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/cancel`, {
      method: 'POST',
      credentials: 'include',
    });
  } catch {
    // Best-effort by nature: offline, or the route answered badly. There is nothing the user
    // can do about it and nothing to show them — the turn ends at its step cap either way, and
    // an unhandled rejection here would be reported as a fault they did not cause.
  }
}

/**
 * Send a message and stream the turn. `onEvent` receives every frame in order,
 * ending with exactly one `result`. The returned promise resolves when the stream
 * ends — including when it was cancelled, which is a normal outcome rather than
 * an error the user must act on.
 */
export function sendTurn(sessionId: string, text: string, onEvent: (e: TurnEvent) => void): Turn {
  return streamTurn(
    sessionId,
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
    sessionId,
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
    sessionId,
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/opening`,
    {},
    'could not open the rehearsal',
    onEvent,
  );
}

/**
 * Resume after a failed turn without re-sending the user's message. The server continues
 * from the existing transcript (healing any dangling tool calls), so the model's context
 * is not polluted with a duplicate prompt.
 */
export function retryTurn(sessionId: string, onEvent: (e: TurnEvent) => void): Turn {
  return streamTurn(
    sessionId,
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/retry`,
    {},
    'could not retry the turn',
    onEvent,
  );
}

/** A turn the plan would not allow.
 *
 *  It carries what the server sent rather than only a sentence, because the two refusals a
 *  candidate can meet have different remedies: a spent daily allowance means come back
 *  tomorrow, while a session that reached its turn ceiling can be continued right now by
 *  spending another of the day's sessions. `canExtend` is which one this is. */
export class TurnRefused extends Error {
  readonly sessionId: string;
  readonly canExtend: boolean;
  readonly turns: number;
  readonly ceiling: number;

  constructor(
    message: string,
    sessionId: string,
    canExtend: boolean,
    turns: number,
    ceiling: number,
  ) {
    super(message);
    this.name = 'TurnRefused';
    this.sessionId = sessionId;
    this.canExtend = canExtend;
    this.turns = turns;
    this.ceiling = ceiling;
  }
}

/** What to throw when a turn does not start.
 *
 *  A refused turn (402) already carries a sentence written for the candidate — which
 *  feature ran out and when it comes back — so that is passed through verbatim, along with
 *  whether this particular refusal can be lifted by extending the session.
 *
 *  Reading the body cannot be allowed to replace one failure with another: a refusal whose
 *  body is unreadable still has to surface as the refusal it is. */
async function turnFailure(res: Response, sessionId: string, failure: string): Promise<Error> {
  if (res.status !== 402) {
    return new Error(`${failure} (${res.status})`);
  }
  try {
    const body = await res.json();
    const session = body?.session ?? {};
    return new TurnRefused(
      typeof body?.error === 'string' ? body.error : `${failure} (402)`,
      sessionId,
      body?.can_extend === true,
      typeof session.turns === 'number' ? session.turns : 0,
      typeof session.ceiling === 'number' ? session.ceiling : 0,
    );
  } catch {
    return new TurnRefused(`${failure} (402)`, sessionId, false, 0, 0);
  }
}

/** Buy this tailoring session another ceiling's worth of turns, out of the day's tailoring
 *  allowance. Rejects with a TurnRefused when there is none left to spend. */
export async function extendSession(sessionId: string): Promise<void> {
  const res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/extend`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!res.ok) {
    throw await turnFailure(res, sessionId, 'could not continue this session');
  }
}

/** POST a turn and stream its frames. Shared by every way of starting one, so cancellation
 *  and frame decoding have a single implementation. */
function streamTurn(
  sessionId: string,
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
      throw await turnFailure(res, sessionId, failure);
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
      // Anything else broke the stream, not the turn. The caller re-reads the session.
      throw new StreamInterrupted(e);
    }
  })();

  return {
    done,
    cancel: () => {
      // Both halves: tell the server to stop the work, and stop reading it here. The
      // server no longer infers the first from the second. cancelTurn swallows its own
      // failures, so this cannot reject.
      void cancelTurn(sessionId);
      controller.abort();
    },
  };
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
