// The status vocabulary as the inbox landing page explains it: one entry per
// signal the classifier can label an email with, in pipeline order. The labels
// and badge colours come from `emailStatus.ts` — the same source the real inbox
// renders from — so the page shows the product's own chips, and a test pins this
// list to that vocabulary so a new signal can't leave the page silently partial.
//
// `other` is deliberately absent: it is the sanitizer's fallback for anything
// out of vocabulary, not a status worth advertising. The page says so in prose.

export interface InboxStatusEntry {
  signal: string;
  description: string;
}

export const INBOX_STATUS_GUIDE: InboxStatusEntry[] = [
  { signal: 'acknowledgement', description: 'They got your application.' },
  { signal: 'screening', description: 'A recruiter screen or first call.' },
  { signal: 'assessment', description: 'A take-home or a test to complete.' },
  { signal: 'interview_invitation', description: 'An interview is on the table.' },
  { signal: 'offer', description: 'An offer arrived.' },
  { signal: 'rejection', description: 'A no — recorded, never guessed at.' },
  { signal: 'info_request', description: 'They need something from you.' },
  { signal: 'incomplete_application', description: 'You started an application and never sent it.' },
];
