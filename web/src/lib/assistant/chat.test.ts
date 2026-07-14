import { describe, it, expect } from 'vitest';
import { initChat, reduceTurnEvent } from './chat';
import type { TurnEvent } from './wire';

describe('reduceTurnEvent', () => {
  it('accumulates assistant_text chunks into one message', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Hello' });
    s = reduceTurnEvent(s, { type: 'assistant_text', text: ', world' });
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]?.role).toBe('assistant');
    expect(s.messages[0]?.text).toBe('Hello, world');
    expect(s.messages[0]?.streaming).toBe(true);
  });

  it('keeps assistant_thought separate from the reply text', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_thought', text: 'Let me think. ' });
    s = reduceTurnEvent(s, { type: 'assistant_thought', text: 'OK.' });
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'The answer.' });
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]?.thinking).toBe('Let me think. OK.');
    expect(s.messages[0]?.text).toBe('The answer.');
  });

  it('records tool_use calls with their name and input on the current assistant message', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'tool_use', name: 'read_file', input: {} });
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Done' });
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]?.tools).toEqual([{ name: 'read_file', input: {} }]);
  });

  it('captures the tool input so tool cards can render details', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'tool_use', name: 'Bash', input: { command: 'ls -la' } });
    s = reduceTurnEvent(s, { type: 'tool_use', name: 'Read', input: { file_path: '/tmp/x.ts' } });
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]?.tools).toEqual([
      { name: 'Bash', input: { command: 'ls -la' } },
      { name: 'Read', input: { file_path: '/tmp/x.ts' } },
    ]);
  });

  it('appends a user message for a user_prompt event', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'user_prompt', text: 'Hi there' });
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]?.role).toBe('user');
    expect(s.messages[0]?.text).toBe('Hi there');
  });

  it('starts a fresh assistant message after a user prompt', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'user_prompt', text: 'Question?' });
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Answer.' });
    expect(s.messages).toHaveLength(2);
    expect(s.messages[0]?.role).toBe('user');
    expect(s.messages[1]?.role).toBe('assistant');
  });

  it('closes the turn on a terminal result event', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Reply' });
    s = reduceTurnEvent(s, {
      type: 'result',
      cost_usd: 0.01,
      stop_reason: 'end_turn',
      is_error: false,
    });
    expect(s.messages[0]?.streaming).toBe(false);
    expect(s.messages[0]?.errored).toBe(false);
  });

  it('marks the message errored when the turn ends with an error', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'partial' });
    s = reduceTurnEvent(s, { type: 'result', cost_usd: null, stop_reason: 'error', is_error: true });
    expect(s.messages[0]?.streaming).toBe(false);
    expect(s.messages[0]?.errored).toBe(true);
  });

  it('opens a new assistant message for the next turn after a result', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'First' });
    s = reduceTurnEvent(s, { type: 'result', cost_usd: null, stop_reason: 'end_turn', is_error: false });
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Second' });
    expect(s.messages).toHaveLength(2);
    expect(s.messages[1]?.text).toBe('Second');
    expect(s.messages[1]?.streaming).toBe(true);
  });

  it('ignores a result with no open turn instead of creating a message', () => {
    const s = reduceTurnEvent(initChat(), {
      type: 'result',
      cost_usd: null,
      stop_reason: 'end_turn',
      is_error: false,
    });
    expect(s.messages).toHaveLength(0);
  });

  it('ignores unmodeled/raw event kinds without throwing', () => {
    let s = initChat();
    s = reduceTurnEvent(s, { type: 'assistant_text', text: 'Hi' });
    const before = s;
    s = reduceTurnEvent(s, { type: 'raw', value: { anything: true } });
    s = reduceTurnEvent(s, { type: 'usage', input_tokens: 1, output_tokens: 2, cost_usd: null });
    s = reduceTurnEvent(s, { type: 'system', subtype: 'init' });
    // An unknown kind not in the union must also be a no-op, not a throw.
    s = reduceTurnEvent(s, { type: 'totally_unknown' } as unknown as TurnEvent);
    expect(s).toBe(before);
    expect(s.messages).toHaveLength(1);
  });

  it('does not mutate the previous state', () => {
    const prev = initChat();
    const next = reduceTurnEvent(prev, { type: 'user_prompt', text: 'x' });
    expect(prev.messages).toHaveLength(0);
    expect(next).not.toBe(prev);
  });
});
