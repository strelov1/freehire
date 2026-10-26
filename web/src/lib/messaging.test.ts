import { describe, expect, it } from 'vitest';
import { conversationStatusLabel, counterpartName, followsNewMessages, isSendShortcut, mergeMessages, reportReasonLabel } from './messaging';

describe('message list scrolling', () => {
  it('follows new messages only while the reader is at the bottom', () => {
    expect(followsNewMessages({ scrollTop: 600, scrollHeight: 1000, clientHeight: 400 })).toBe(true);
    expect(followsNewMessages({ scrollTop: 560, scrollHeight: 1000, clientHeight: 400 })).toBe(true);
    expect(followsNewMessages({ scrollTop: 500, scrollHeight: 1000, clientHeight: 400 })).toBe(false);
    expect(followsNewMessages({ scrollTop: 0, scrollHeight: 300, clientHeight: 400 })).toBe(true);
  });
});

describe('send shortcut', () => {
  it('sends on Cmd or Ctrl+Enter, never a plain Enter or a composing key', () => {
    expect(isSendShortcut({ key: 'Enter', metaKey: true, ctrlKey: false })).toBe(true);
    expect(isSendShortcut({ key: 'Enter', metaKey: false, ctrlKey: true })).toBe(true);
    expect(isSendShortcut({ key: 'Enter', metaKey: false, ctrlKey: false })).toBe(false);
    expect(isSendShortcut({ key: 'Enter', metaKey: true, ctrlKey: false, isComposing: true })).toBe(false);
    expect(isSendShortcut({ key: 'a', metaKey: true, ctrlKey: false })).toBe(false);
  });
});

describe('conversation presentation', () => {
  it('never reveals a declined invitation to its sender', () => {
    expect(conversationStatusLabel('outgoing')).toBe('Waiting for a reply');
  });

  it('names a buddy by handle, and an unreachable one without any', () => {
    expect(counterpartName({ persona: 'buddy', reachable: false, detail: null })).toBe('Former member');
    expect(counterpartName({ persona: 'buddy', reachable: true, detail: { handle: 'buddy-123', card_summary: { category: '', seniority: '', country: '' } } })).toBe('buddy-123');
  });

  it('labels report reasons for moderators', () => {
    expect((['spam', 'harassment', 'scam', 'other'] as const).map(reportReasonLabel)).toEqual([
      'Spam', 'Harassment', 'Scam', 'Other',
    ]);
  });

  it('merges polled messages by id in chronological order', () => {
    expect(mergeMessages([{ id: 2 }, { id: 3 }], [{ id: 3 }, { id: 1 }])).toEqual([
      { id: 1 }, { id: 2 }, { id: 3 },
    ]);
  });
});
