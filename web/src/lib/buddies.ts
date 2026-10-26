import type { Conversation } from './types.js';

/** What the "Message" control does for one member, from the buddy conversation already
 *  held with them (one per pair): none yet, or one closed by a leave, asks for a first
 *  message; any other goes to it. A counterpart who is unreachable (left, blocked) carries
 *  no handle, so their conversation never matches a member shown anywhere. */
type BuddyMessageAction =
  | { kind: 'invite' }
  | { kind: 'open' | 'sent' | 'reply'; conversationId: number };

export function messageActionFor(conversations: readonly Pick<Conversation, 'id' | 'counterpart' | 'status_for_me'>[], handle: string): BuddyMessageAction {
  const conv = conversations.find(({ counterpart: c }) => c.persona === 'buddy' && c.reachable && c.detail.handle === handle);
  switch (conv?.status_for_me) {
    case 'open': return { kind: 'open', conversationId: conv.id };
    // A declined invitation reads as outgoing too: the sender is never told.
    case 'outgoing': return { kind: 'sent', conversationId: conv.id };
    case 'incoming': return { kind: 'reply', conversationId: conv.id };
    // Closed by a leave: a new invitation reopens it.
    default: return { kind: 'invite' };
  }
}

const MESSAGE_ACTION_LABELS = {
  invite: 'Message',
  open: 'Message',
  sent: 'Invitation sent',
  reply: 'Reply to invitation',
} as const;

export function messageActionLabel(kind: keyof typeof MESSAGE_ACTION_LABELS): string {
  return MESSAGE_ACTION_LABELS[kind];
}

const FEED_EVENT_LABELS = {
  applied: 'Applied to',
  interview: 'Interviewing at',
  offer: 'Got an offer from',
} as const;

export function buddyFeedEventLabel(type: keyof typeof FEED_EVENT_LABELS): string {
  return FEED_EVENT_LABELS[type];
}
