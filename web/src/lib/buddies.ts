const STATUS_LABELS = {
  incoming: 'Invitation',
  outgoing: 'Waiting for a reply',
  open: 'Chat',
  closed: 'Closed',
} as const;

export function buddyOutgoingLabel(status: keyof typeof STATUS_LABELS): string {
  return STATUS_LABELS[status];
}

export function buddyConversationName(formerMember: boolean, handle: string): string {
  return formerMember ? 'Former member' : handle;
}

const REPORT_REASON_LABELS = {
  spam: 'Spam',
  harassment: 'Harassment',
  scam: 'Scam',
  other: 'Other',
} as const;

export function buddyReportReasonLabel(reason: keyof typeof REPORT_REASON_LABELS): string {
  return REPORT_REASON_LABELS[reason];
}

export function mergeBuddyMessages<T extends { id: number }>(current: T[], incoming: T[]): T[] {
  const byId = new Map(current.map((message) => [message.id, message]));
  for (const message of incoming) byId.set(message.id, message);
  return [...byId.values()].sort((a, b) => a.id - b.id);
}

/** What the "Message" control does for one member, from the conversation already held
 *  with them (one per pair). A former member's row carries an empty handle, so it never
 *  matches a live one. */
export type BuddyMessageAction =
  | { kind: 'invite' }
  | { kind: 'open' | 'sent' | 'reply'; conversationId: number }
  | { kind: 'none' };

interface ConversationLike {
  id: number;
  other: { handle: string };
  status_for_me: 'incoming' | 'outgoing' | 'open' | 'closed';
  former_member?: boolean;
}

export function messageActionFor(conversations: readonly ConversationLike[], handle: string): BuddyMessageAction {
  const conv = handle ? conversations.find((c) => c.other.handle === handle) : undefined;
  if (!conv) return { kind: 'invite' };
  switch (conv.status_for_me) {
    case 'open': return { kind: 'open', conversationId: conv.id };
    // A declined invitation reads as outgoing too: the sender is never told.
    case 'outgoing': return { kind: 'sent', conversationId: conv.id };
    case 'incoming': return { kind: 'reply', conversationId: conv.id };
    // Closed by a leave: a new invitation reopens it. A former member (left or blocked)
    // cannot be invited.
    case 'closed': return conv.former_member ? { kind: 'none' } : { kind: 'invite' };
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
