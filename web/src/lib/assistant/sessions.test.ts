import { describe, it, expect } from 'vitest';
import {
  labelFromMessage,
  resolveLabel,
  fromSummary,
  newestFirst,
  upsertSession,
  removeSession,
  setLabel,
  activeAfterDelete,
  type SessionItem,
  type SessionSummary,
} from './sessions';

const item = (id: string, createdAt: number, extra: Partial<SessionItem> = {}): SessionItem => ({
  id,
  label: id,
  createdAt,
  live: false,
  ...extra,
});

const summary = (over: Partial<SessionSummary> = {}): SessionSummary => ({
  session_id: 's1',
  display_label: null,
  created_at: 100,
  live: false,
  ...over,
});

describe('labelFromMessage', () => {
  it('collapses whitespace and trims', () => {
    expect(labelFromMessage('  hello   there\n world ')).toBe('hello there world');
  });

  it('truncates long text with an ellipsis', () => {
    const long = 'a'.repeat(80);
    const label = labelFromMessage(long);
    expect(label.length).toBeLessThanOrEqual(60);
    expect(label.endsWith('…')).toBe(true);
  });

  it('leaves short text intact', () => {
    expect(labelFromMessage('short')).toBe('short');
  });
});

describe('resolveLabel', () => {
  it('prefers a derived/cached label over everything', () => {
    expect(resolveLabel(summary({ display_label: 'backend' }), 'derived', 'fallback')).toBe(
      'derived',
    );
  });

  it('falls back to backend display_label when no cached label', () => {
    expect(resolveLabel(summary({ display_label: 'backend' }), undefined, 'fallback')).toBe(
      'backend',
    );
  });

  it('uses the fallback when neither cached nor backend label is present', () => {
    expect(resolveLabel(summary({ display_label: null }), undefined, 'fallback')).toBe('fallback');
    // blank strings are ignored, not shown as an empty label
    expect(resolveLabel(summary({ display_label: '  ' }), '  ', 'fallback')).toBe('fallback');
  });
});

describe('fromSummary', () => {
  it('maps a wire row to a session item using resolveLabel', () => {
    const it = fromSummary(summary({ session_id: 'x', created_at: 42, live: true }), undefined, 'fb');
    expect(it).toEqual({ id: 'x', label: 'fb', createdAt: 42, live: true });
  });
});

describe('newestFirst', () => {
  it('sorts by createdAt descending without mutating the input', () => {
    const input = [item('a', 1), item('b', 3), item('c', 2)];
    const sorted = newestFirst(input);
    expect(sorted.map((i) => i.id)).toEqual(['b', 'c', 'a']);
    expect(input.map((i) => i.id)).toEqual(['a', 'b', 'c']);
  });
});

describe('upsertSession', () => {
  it('adds a new session at the top when it is the newest', () => {
    const items = [item('a', 1), item('b', 2)];
    const out = upsertSession(items, item('c', 3));
    expect(out.map((i) => i.id)).toEqual(['c', 'b', 'a']);
  });

  it('replaces an existing session by id (no duplicate) and re-sorts', () => {
    const items = [item('a', 1), item('b', 2)];
    const out = upsertSession(items, item('a', 5, { label: 'renamed' }));
    expect(out.map((i) => i.id)).toEqual(['a', 'b']);
    expect(out.find((i) => i.id === 'a')?.label).toBe('renamed');
  });
});

describe('removeSession', () => {
  it('drops the matching id', () => {
    expect(removeSession([item('a', 1), item('b', 2)], 'a').map((i) => i.id)).toEqual(['b']);
  });

  it('is a no-op for an unknown id', () => {
    expect(removeSession([item('a', 1)], 'zzz').map((i) => i.id)).toEqual(['a']);
  });
});

describe('setLabel', () => {
  it('updates only the targeted session label', () => {
    const out = setLabel([item('a', 1), item('b', 2)], 'b', 'New');
    expect(out.find((i) => i.id === 'b')?.label).toBe('New');
    expect(out.find((i) => i.id === 'a')?.label).toBe('a');
  });
});

describe('activeAfterDelete', () => {
  it('keeps the current active when a non-active session is deleted', () => {
    const remaining = [item('a', 1), item('b', 2)];
    expect(activeAfterDelete(remaining, false, 'a')).toBe('a');
  });

  it('activates the newest remaining when the active session is deleted', () => {
    const remaining = [item('a', 1), item('b', 3), item('c', 2)];
    expect(activeAfterDelete(remaining, true, 'deleted')).toBe('b');
  });

  it('returns null when the last session is deleted', () => {
    expect(activeAfterDelete([], true, 'deleted')).toBeNull();
  });
});
