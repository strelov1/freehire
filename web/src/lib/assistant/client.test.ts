import { describe, expect, it, vi, afterEach } from 'vitest';

import { sendTurn, StreamInterrupted } from './client';

/** A response whose body streams the given chunks and then does whatever `end` says. */
function streamingResponse(chunks: string[], end: 'close' | 'fail' = 'close'): Response {
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(new TextEncoder().encode(chunk));
      if (end === 'fail') controller.error(new Error('network went away'));
      else controller.close();
    },
  });
  return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } });
}

function frame(event: Record<string, unknown>): string {
  return `event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('sendTurn', () => {
  it('reports an interrupted stream as an interruption, not a failure', async () => {
    // The turn keeps running on the server and its transcript is stored, so a broken stream
    // says nothing about whether the work succeeded — presenting it as a failed turn would
    // misreport the state of the user's own CV.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(streamingResponse([frame({ type: 'assistant_text', text: 'hi' })], 'fail')),
    );

    const turn = sendTurn('session-1', 'hello', () => {});

    await expect(turn.done).rejects.toBeInstanceOf(StreamInterrupted);
  });

  it('asks the server to stop the turn, rather than just stopping its own reading', async () => {
    // Aborting the read used to be the only way to stop a turn: the server noticed its next
    // write fail. It no longer does, so stopping has to be said out loud.
    const fetchMock = vi.fn().mockResolvedValue(streamingResponse([frame({ type: 'result' })]));
    vi.stubGlobal('fetch', fetchMock);

    const turn = sendTurn('session-1', 'hello', () => {});
    turn.cancel();

    const cancelled = fetchMock.mock.calls.find(([url]) =>
      String(url).endsWith('/assistant/sessions/session-1/cancel'),
    );
    expect(cancelled, 'no request was made to the cancel route').toBeTruthy();
    expect(cancelled?.[1]).toMatchObject({ method: 'POST' });
  });

  it('still reports a refused turn as a failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 409 })));

    const turn = sendTurn('session-1', 'hello', () => {});

    await expect(turn.done).rejects.not.toBeInstanceOf(StreamInterrupted);
  });
});
