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
