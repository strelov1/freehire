import { describe, expect, it } from 'vitest';
import { eventsFromTranscript, type StoredMessage } from './wire';
import { initChat, reduceTurnEvent } from './chat';

const msg = (seq: number, role: StoredMessage['role'], content: unknown): StoredMessage => ({
  seq,
  role,
  content,
});

describe('eventsFromTranscript', () => {
  it('replays a plain exchange as a prompt, an answer and a close', () => {
    const events = eventsFromTranscript([
      msg(1, 'user', { text: 'hi' }),
      msg(2, 'assistant', { text: 'hello' }),
    ]);
    expect(events.map((e) => e.type)).toEqual(['user_prompt', 'assistant_text', 'result']);
  });

  it('replays a tool round in order', () => {
    const events = eventsFromTranscript([
      msg(1, 'user', { text: 'find go jobs' }),
      msg(2, 'assistant', {
        tool_calls: [{ id: 'c1', name: 'search_jobs', arguments: '{"query":"go"}' }],
      }),
      msg(3, 'tool', { tool_call_id: 'c1', name: 'search_jobs', result: '{"total":2}' }),
      msg(4, 'assistant', { text: 'Found two.' }),
    ]);
    expect(events.map((e) => e.type)).toEqual([
      'user_prompt',
      'tool_use',
      'tool_result',
      'assistant_text',
      'result',
    ]);
    const use = events[1];
    expect(use?.type === 'tool_use' ? use.input : null).toEqual({ query: 'go' });
  });

  it('closes the previous turn before the next prompt', () => {
    // Without the synthesised close, the earlier answer would still look like it
    // was streaming after the conversation moved on.
    const events = eventsFromTranscript([
      msg(1, 'user', { text: 'one' }),
      msg(2, 'assistant', { text: 'first' }),
      msg(3, 'user', { text: 'two' }),
      msg(4, 'assistant', { text: 'second' }),
    ]);
    expect(events.map((e) => e.type)).toEqual([
      'user_prompt',
      'assistant_text',
      'result',
      'user_prompt',
      'assistant_text',
      'result',
    ]);
  });

  it('marks a failed tool result from its error envelope', () => {
    const events = eventsFromTranscript([
      msg(1, 'tool', { name: 'facets', result: '{"error":"search is not available"}' }),
    ]);
    const result = events[0];
    expect(result?.type === 'tool_result' ? result.is_error : false).toBe(true);
  });

  it('keeps unparseable tool arguments rather than dropping the call', () => {
    const events = eventsFromTranscript([
      msg(1, 'assistant', { tool_calls: [{ id: 'c', name: 'facets', arguments: 'not json' }] }),
    ]);
    const use = events[0];
    expect(use?.type === 'tool_use' ? use.input : null).toBe('not json');
  });

  it('is empty for an empty transcript', () => {
    expect(eventsFromTranscript([])).toEqual([]);
  });
});

describe('a replayed transcript and a live turn fold the same way', () => {
  it('reconstructs the message list a live turn would have produced', () => {
    const events = eventsFromTranscript([
      msg(1, 'user', { text: 'find go jobs' }),
      msg(2, 'assistant', {
        tool_calls: [{ id: 'c1', name: 'search_jobs', arguments: '{"query":"go"}' }],
      }),
      msg(3, 'tool', { tool_call_id: 'c1', name: 'search_jobs', result: '{"total":2}' }),
      msg(4, 'assistant', { text: 'Found two.' }),
    ]);
    const state = events.reduce(reduceTurnEvent, initChat());

    expect(state.messages).toHaveLength(2);
    expect(state.messages[0]).toMatchObject({ role: 'user', text: 'find go jobs' });
    expect(state.messages[1]).toMatchObject({
      role: 'assistant',
      text: 'Found two.',
      streaming: false,
    });
    expect(state.messages[1]?.tools).toHaveLength(1);
    expect(state.messages[1]?.tools[0]).toMatchObject({
      name: 'search_jobs',
      result: '{"total":2}',
      isError: false,
    });
  });
});
