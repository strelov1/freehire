import { describe, it, expect, vi, afterEach } from 'vitest';
import { sendTurn } from './client';
import type { TurnEvent } from './wire';
import { must } from '../test-utils';

/** A response body that emits the given chunks, then ends. */
function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}

/** A body that emits one chunk and then stays open until the signal aborts it. */
function hangingStream(signal: AbortSignal): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode('event: assistant_text\ndata: {"type":"assistant_text","text":"thinking"}\n\n'));
      signal.addEventListener('abort', () => controller.error(new DOMException('aborted', 'AbortError')));
    },
  });
}

afterEach(() => vi.unstubAllGlobals());

describe('sendTurn', () => {
  it('streams the turn frame by frame, ending with its terminal event', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      status: 200,
      body: streamOf([
        'event: user_prompt\ndata: {"type":"user_prompt","text":"hi"}\n\n',
        'event: assistant_text\ndata: {"type":"assistant_text","text":"hello"}\n\n',
        'event: result\ndata: {"type":"result","stop_reason":"end_turn"}\n\n',
      ]),
    }));

    const seen: TurnEvent[] = [];
    await sendTurn('s1', 'hi', 'tok', (e) => seen.push(e)).done;

    expect(seen.map((e) => e.type)).toEqual(['user_prompt', 'assistant_text', 'result']);
  });

  it('carries the Bearer credential and the message', async () => {
    const calls: { url: string; init: RequestInit }[] = [];
    vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      return { ok: true, status: 200, body: streamOf(['event: result\ndata: {"type":"result"}\n\n']) };
    });

    await sendTurn('s1', 'hello there', 'tok-9', () => {}).done;

    expect(must(calls[0]).init.headers).toMatchObject({ Authorization: 'Bearer tok-9' });
    expect(JSON.parse(must(calls[0]).init.body as string)).toEqual({ text: 'hello there' });
    // `credentials: 'omit'`, not the default: host_permissions makes Chrome attach a
    // signed-in website tab's session cookie to this call otherwise, and hire's auth
    // middleware reads that cookie's mere presence as "not the extension" and 403s.
    expect(must(calls[0]).init.credentials).toBe('omit');
  });

  // Stopping is something the user chose. It has to arrive as a terminal event, or
  // the composer stays disabled waiting for one that will never come.
  it('reports cancelling as a cancelled result, not as a failure', async () => {
    vi.stubGlobal('fetch', async (_url: string, init: RequestInit) => ({
      ok: true,
      status: 200,
      body: hangingStream(init.signal as AbortSignal),
    }));

    const seen: TurnEvent[] = [];
    const turn = sendTurn('s1', 'hi', 'tok', (e) => seen.push(e));
    await new Promise((r) => setTimeout(r, 10));
    turn.cancel();
    await turn.done;

    expect(seen.at(-1)).toEqual({ type: 'result', stop_reason: 'cancelled' });
  });

  // Stop renders as soon as a turn starts, so it is reachable before the response
  // headers arrive. There the abort rejects the fetch, not the stream read.
  it('reports cancelling before the response arrives as cancelled too', async () => {
    vi.stubGlobal('fetch', (_url: string, init: RequestInit) => new Promise((_resolve, reject) => {
      (init.signal as AbortSignal).addEventListener('abort', () =>
        reject(new DOMException('aborted', 'AbortError')));
    }));

    const seen: TurnEvent[] = [];
    const turn = sendTurn('s1', 'hi', 'tok', (e) => seen.push(e));
    turn.cancel();
    await turn.done;

    expect(seen).toEqual([{ type: 'result', stop_reason: 'cancelled' }]);
  });

  it('rejects when the turn could not be started', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 503, body: null }));

    await expect(sendTurn('s1', 'hi', 'tok', () => {}).done).rejects.toThrow();
  });

  // One malformed frame must not abandon a turn that is otherwise fine.
  it('drops a frame it cannot parse and keeps reading', async () => {
    vi.stubGlobal('fetch', async () => ({
      ok: true,
      status: 200,
      body: streamOf([
        'event: assistant_text\ndata: {not json}\n\n',
        'event: result\ndata: {"type":"result","stop_reason":"end_turn"}\n\n',
      ]),
    }));

    const seen: TurnEvent[] = [];
    await sendTurn('s1', 'hi', 'tok', (e) => seen.push(e)).done;

    expect(seen.map((e) => e.type)).toEqual(['result']);
  });
});
