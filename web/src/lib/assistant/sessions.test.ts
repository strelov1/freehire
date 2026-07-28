import { describe, expect, it } from 'vitest';
import {
  activeAfterDelete,
  fromSummary,
  labelFromMessage,
  removeSession,
  setLabel,
  upsertSession,
  type SessionItem,
} from './sessions';
import type { SessionSummary } from './wire';

const item = (id: string, label = id): SessionItem => ({ id, label, preset: 'chat' });

describe('labelFromMessage', () => {
  it('collapses whitespace to one line', () => {
    expect(labelFromMessage('  find   go\njobs ')).toBe('find go jobs');
  });

  it('truncates a long message with an ellipsis', () => {
    const label = labelFromMessage('x'.repeat(200));
    expect(label).toHaveLength(60);
    expect(label.endsWith('…')).toBe(true);
  });
});

describe('fromSummary', () => {
  const summary = (label: string): SessionSummary => ({ id: '7', preset: 'chat', label });

  it('names a session after its first message', () => {
    expect(fromSummary(summary('find go jobs'), 'New chat').label).toBe('find go jobs');
  });

  it('falls back when the conversation has no turns yet', () => {
    // The backend labels a session on its first turn, so a just-created one is blank.
    expect(fromSummary(summary(''), 'New chat').label).toBe('New chat');
    expect(fromSummary(summary('   '), 'New chat').label).toBe('New chat');
  });

  it('carries the preset so the rail can tell a tailoring chat apart', () => {
    expect(fromSummary({ id: '1', preset: 'tailor', label: 'CV' }, 'x').preset).toBe('tailor');
  });
});

describe('the list reducers', () => {
  it('puts a new session at the top', () => {
    expect(upsertSession([item('a')], item('b')).map((i) => i.id)).toEqual(['b', 'a']);
  });

  it('replaces rather than duplicates an existing id', () => {
    const updated = upsertSession([item('a', 'old'), item('b')], item('a', 'new'));
    expect(updated.map((i) => i.id)).toEqual(['a', 'b']);
    expect(updated[0]?.label).toBe('new');
  });

  it('removes by id and ignores an unknown one', () => {
    expect(removeSession([item('a'), item('b')], 'a').map((i) => i.id)).toEqual(['b']);
    expect(removeSession([item('a')], 'zz').map((i) => i.id)).toEqual(['a']);
  });

  it('relabels one session only', () => {
    const relabelled = setLabel([item('a'), item('b')], 'a', 'renamed');
    expect(relabelled[0]?.label).toBe('renamed');
    expect(relabelled[1]?.label).toBe('b');
  });
});

describe('activeAfterDelete', () => {
  it('keeps the active session when another was deleted', () => {
    expect(activeAfterDelete([item('a')], false, 'a')).toBe('a');
  });

  it('activates the newest remaining when the active one was deleted', () => {
    expect(activeAfterDelete([item('b'), item('c')], true, 'a')).toBe('b');
  });

  it('reports none left, so the caller can start a fresh chat', () => {
    expect(activeAfterDelete([], true, 'a')).toBeNull();
  });
});
