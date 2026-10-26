import { describe, expect, it } from 'vitest';
import { buddyFeedEventLabel, messageActionFor, messageActionLabel } from './buddies';
import type { Conversation } from './types';

describe('buddy member actions', () => {
  const summary = { category: '', seniority: '', country: '' };
  const conv = (id: number, handle: string, status_for_me: Conversation['status_for_me']) =>
    ({ id, status_for_me, counterpart: { persona: 'buddy', reachable: true, detail: { handle, card_summary: summary } } } as const);
  const gone = (id: number, status_for_me: Conversation['status_for_me']) =>
    ({ id, status_for_me, counterpart: { persona: 'buddy', reachable: false, detail: null } } as const);

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

  it('never matches an unreachable counterpart, whose handle is not sent', () => {
    expect(messageActionFor([gone(11, 'open'), gone(12, 'closed')], 'buddy-1')).toEqual({ kind: 'invite' });
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
