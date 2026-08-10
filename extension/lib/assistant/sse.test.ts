import { describe, expect, it } from 'vitest';
import { parseFrames } from './sse';

describe('parseFrames', () => {
  it('parses a complete frame', () => {
    const { frames, rest } = parseFrames('event: assistant_text\ndata: {"text":"hi"}\n\n');
    expect(frames).toEqual([{ event: 'assistant_text', data: '{"text":"hi"}' }]);
    expect(rest).toBe('');
  });

  it('holds a partial frame back until the rest arrives', () => {
    // A chunk boundary can fall anywhere; emitting half a frame would feed the
    // reducer a truncated event.
    const first = parseFrames('event: assistant_text\ndata: {"te');
    expect(first.frames).toEqual([]);

    const second = parseFrames(first.rest + 'xt":"hi"}\n\n');
    expect(second.frames).toEqual([{ event: 'assistant_text', data: '{"text":"hi"}' }]);
  });

  it('parses several frames from one chunk', () => {
    const { frames } = parseFrames(
      'event: tool_use\ndata: {"name":"facets"}\n\nevent: result\ndata: {"stop_reason":"end_turn"}\n\n',
    );
    expect(frames.map((f) => f.event)).toEqual(['tool_use', 'result']);
  });

  it('ignores keep-alive comments', () => {
    const { frames } = parseFrames(': keepalive\n\nevent: result\ndata: {}\n\n');
    expect(frames).toEqual([{ event: 'result', data: '{}' }]);
  });

  it('joins multi-line data', () => {
    const { frames } = parseFrames('event: x\ndata: one\ndata: two\n\n');
    expect(frames[0]?.data).toBe('one\ntwo');
  });
});
