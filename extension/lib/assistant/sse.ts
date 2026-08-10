// A minimal SSE reader for the assistant's turn stream.
//
// `EventSource` cannot be used here: a turn is a POST (the message is its body)
// and EventSource only issues GETs. So the stream is read off a `fetch` response
// body, which also gives cancellation for free — aborting the fetch is what tells
// the backend the user left, and the backend stops the turn on its next boundary.

/** One parsed SSE frame: its event name and its raw data payload. */
export interface SSEFrame {
  event: string;
  data: string;
}

/**
 * Split a buffer into complete SSE frames, returning them plus whatever partial
 * frame is left over. Frames are separated by a blank line; comment lines (`:`)
 * are the keep-alive and carry nothing. Kept pure so the framing is testable
 * without a network.
 */
export function parseFrames(buffer: string): { frames: SSEFrame[]; rest: string } {
  const frames: SSEFrame[] = [];
  let rest = buffer;

  for (;;) {
    const boundary = rest.indexOf('\n\n');
    if (boundary === -1) break;
    const block = rest.slice(0, boundary);
    rest = rest.slice(boundary + 2);

    let event = 'message';
    const data: string[] = [];
    for (const line of block.split('\n')) {
      if (line.startsWith(':')) continue; // keep-alive comment
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) data.push(line.slice(5).trim());
    }
    if (data.length > 0) frames.push({ event, data: data.join('\n') });
  }
  return { frames, rest };
}

/**
 * Read an SSE response body, invoking `onFrame` for each complete frame. Returns
 * when the stream ends. An aborted fetch surfaces as a rejected read, which the
 * caller treats as "the user cancelled", not as a failure.
 */
export async function readFrames(
  body: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const { frames, rest } = parseFrames(buffer);
    buffer = rest;
    for (const frame of frames) onFrame(frame);
  }
}
