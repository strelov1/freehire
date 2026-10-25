import { describe, expect, it } from 'vitest';
import { buddyConversationName, buddyFeedEventLabel, buddyOutgoingLabel, buddyReportReasonLabel, followsNewMessages, isSendShortcut, mergeBuddyMessages, messageActionFor, messageActionLabel } from './buddies';

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

describe('buddy member actions', () => {
  const conv = (id: number, handle: string, status_for_me: 'incoming' | 'outgoing' | 'open' | 'closed') => ({ id, other: { handle }, status_for_me });

  it('invites a member with no conversation yet', () => {
    expect(messageActionFor([conv(1, 'someone-else', 'open')], 'buddy-1')).toEqual({ kind: 'invite' });
  });

  it('opens an open conversation', () => {
    expect(messageActionFor([conv(7, 'buddy-1', 'open')], 'buddy-1')).toEqual({ kind: 'open', conversationId: 7 });
  });

  it('shows a pending invitation I sent', () => {
    expect(messageActionFor([conv(8, 'buddy-1', 'outgoing')], 'buddy-1')).toEqual({ kind: 'sent', conversationId: 8 });
  });

  it('offers to reply to an invitation they sent', () => {
    expect(messageActionFor([conv(9, 'buddy-1', 'incoming')], 'buddy-1')).toEqual({ kind: 'reply', conversationId: 9 });
  });

  it('invites again when the conversation was closed by a leave', () => {
    expect(messageActionFor([conv(10, 'buddy-1', 'closed')], 'buddy-1')).toEqual({ kind: 'invite' });
  });

  it('hides the action for a closed conversation with a former member', () => {
    const closed = { ...conv(10, 'buddy-1', 'closed'), former_member: true };
    expect(messageActionFor([closed], 'buddy-1')).toEqual({ kind: 'none' });
  });

  it('never matches a former member by their empty handle', () => {
    expect(messageActionFor([conv(11, '', 'closed')], '')).toEqual({ kind: 'invite' });
  });

  it('words the three feed events', () => {
    expect((['applied', 'interview', 'offer'] as const).map(buddyFeedEventLabel)).toEqual([
      'Applied to', 'Interviewing at', 'Got an offer from',
    ]);
  });

  it('labels each action', () => {
    expect((['invite', 'open', 'sent', 'reply'] as const).map(messageActionLabel)).toEqual([
      'Message', 'Message', 'Invitation sent', 'Reply to invitation',
    ]);
  });
});
