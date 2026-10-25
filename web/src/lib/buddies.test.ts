import { describe, expect, it } from 'vitest';
import { buddyConversationName, buddyOutgoingLabel, buddyReportReasonLabel, mergeBuddyMessages } from './buddies';

describe('buddy conversation presentation', () => {
  it('never reveals a declined invitation to its sender', () => {
    expect(buddyOutgoingLabel('outgoing')).toBe('Waiting for a reply');
  });

  it('labels former members without exposing a handle', () => {
    expect(buddyConversationName(true, '')).toBe('Former member');
    expect(buddyConversationName(false, 'buddy-123')).toBe('buddy-123');
  });

  it('labels report reasons for moderators', () => {
    expect((['spam', 'harassment', 'scam', 'other'] as const).map(buddyReportReasonLabel)).toEqual([
      'Spam', 'Harassment', 'Scam', 'Other',
    ]);
  });

  it('merges polled messages by id in chronological order', () => {
    expect(mergeBuddyMessages([{ id: 2 }, { id: 3 }], [{ id: 3 }, { id: 1 }])).toEqual([
      { id: 1 }, { id: 2 }, { id: 3 },
    ]);
  });
});
