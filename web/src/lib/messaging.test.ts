import { describe, expect, it } from 'vitest';
import { conversationStatusLabel, counterpartName, followsNewMessages, isSendShortcut, mergeMessages, messageDays, messageJobSlugs, reportReasonLabel } from './messaging';

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

describe('messageJobSlugs', () => {
  it('finds the job pages a message links to, on this site, once each', () => {
    const body = 'Here: https://freehire.me/jobs/staff-go-x1 and also freehire.me/jobs/staff-go-x1?ref=chat, plus /jobs/backend-y2.';
    expect(messageJobSlugs(body)).toEqual(['staff-go-x1', 'backend-y2']);
  });
  it('finds a link wrapped in brackets or quotes', () => {
    expect(messageJobSlugs('Look (https://freehire.me/jobs/staff-go-x1) and "/jobs/backend-y2"')).toEqual(['staff-go-x1', 'backend-y2']);
  });
  it('ignores other sites and other pages', () => {
    expect(messageJobSlugs('https://example.com/jobs/abc https://freehire.me/companies/gitlab')).toEqual([]);
  });
  it('previews at most two, so a pasted list does not become a wall of cards', () => {
    expect(messageJobSlugs('/jobs/a1 /jobs/b2 /jobs/c3')).toEqual(['a1', 'b2']);
  });
});

describe('messageDays', () => {
  it('groups messages under the local day they were sent, oldest first as the log reads', () => {
    const at = (d: string) => new Date(d).toISOString();
    const days = messageDays([
      { id: 1, created_at: at('2026-10-29T10:00:00') },
      { id: 2, created_at: at('2026-10-29T18:00:00') },
      { id: 3, created_at: at('2026-10-30T09:00:00') },
    ]);
    expect(days.map((d) => [d.day, d.messages.map((m) => m.id)])).toEqual([
      ['2026-10-29', [1, 2]],
      ['2026-10-30', [3]],
    ]);
  });
});
