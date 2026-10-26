// Presentation of conversations from the messaging core, for every kind and persona. Kept
// free of Svelte so it unit-tests in plain node; the dock's window arithmetic is
// `messagingDock.ts`, and what is buddy-only (the "Message" control) is `buddies.ts`.

import type { Conversation, ConversationReport, ConversationStatus, Counterpart } from './types.js';

const STATUS_LABELS = {
  incoming: 'Invitation',
  outgoing: 'Waiting for a reply',
  open: 'Chat',
  closed: 'Closed',
} as const satisfies Record<ConversationStatus, string>;

export function conversationStatusLabel(status: ConversationStatus): string {
  return STATUS_LABELS[status];
}

/** What the other side is called in text (headers, labels, the screen-reader name), from
 *  the persona they speak as. An unreachable counterpart carries no name at all. */
export function counterpartName(counterpart: Counterpart): string {
  switch (counterpart.persona) {
    case 'buddy': return counterpart.reachable ? counterpart.detail.handle : 'Former member';
  }
}

/** The wording that belongs to a conversation's kind: why a closed one closed, and what a
 *  block does. */
interface KindCopy {
  /** Closed while the other side is still reachable. */
  closed: string;
  /** Closed because the other side is no longer reachable. */
  gone: string;
  block: string;
}

const KIND_COPY = {
  buddy: {
    closed: 'This conversation closed when one of you left interview buddies.',
    gone: 'This member is no longer in interview buddies, so the conversation is closed.',
    block: 'The conversation closes for good, and you stop seeing each other in lists, the feed and invitations. This cannot be undone.',
  },
} as const satisfies Record<Conversation['kind'], KindCopy>;

export function kindCopy(kind: Conversation['kind']): KindCopy {
  return KIND_COPY[kind];
}

const REPORT_REASON_LABELS = {
  spam: 'Spam',
  harassment: 'Harassment',
  scam: 'Scam',
  other: 'Other',
} as const satisfies Record<ConversationReport['reason'], string>;

export function reportReasonLabel(reason: ConversationReport['reason']): string {
  return REPORT_REASON_LABELS[reason];
}

export function mergeMessages<T extends { id: number }>(current: T[], incoming: T[]): T[] {
  const byId = new Map(current.map((message) => [message.id, message]));
  for (const message of incoming) byId.set(message.id, message);
  return [...byId.values()].sort((a, b) => a.id - b.id);
}

/** How close to the bottom (px) the message list must be for new messages to keep it
 *  pinned there. A reader scrolled further up is reading history and stays put. */
const FOLLOW_SLACK_PX = 48;

export function followsNewMessages(list: { scrollTop: number; scrollHeight: number; clientHeight: number }): boolean {
  return list.scrollHeight - list.scrollTop - list.clientHeight <= FOLLOW_SLACK_PX;
}

/** Cmd+Enter or Ctrl+Enter sends; a plain Enter is a newline, and a key pressed while an
 *  input method is composing belongs to the composition. */
export function isSendShortcut(e: { key: string; metaKey: boolean; ctrlKey: boolean; isComposing?: boolean }): boolean {
  return e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !e.isComposing;
}
