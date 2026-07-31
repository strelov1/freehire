// View logic for the board's follow-up affordance: whether a card offers a draft,
// how it reports a chase that already happened, and what lands in the clipboard.
//
// The chase is a SECOND reading, never a replacement. `followed_up_at` is outside
// the server's last-activity derivation on purpose (see migration 0059): silence
// measures how long the employer has been quiet, and the candidate chasing is not
// a reply. A chased card therefore keeps its "24d" badge and adds "chased 2d ago".
import type { MyJob, FollowUpDraft } from './types';

/** Whether the card offers a follow-up draft. Mirrors the server's gate exactly —
 *  it refuses anything whose silence state is not `silent` — so the button can
 *  never be shown where the request would 409. Having already chased does not
 *  withdraw the offer: the employer still has not replied. */
export function canFollowUp(item: MyJob): boolean {
  return item.silence_state === 'silent';
}

/** How the card reports a chase, or null for an application never chased (and for
 *  a timestamp that will not parse — a missing line beats "chased NaNd ago").
 *  Whole days, floored at zero, matching the silence badge's unit so the two
 *  readings sit side by side without inviting arithmetic between them. */
export function chasedLabel(item: MyJob, now: Date = new Date()): string | null {
  if (!item.followed_up_at) return null;
  const at = new Date(item.followed_up_at);
  if (Number.isNaN(at.getTime())) return null;
  const days = Math.floor((now.getTime() - at.getTime()) / 86_400_000);
  // A browser clock ahead of the server would otherwise print a negative age.
  return days <= 0 ? 'chased today' : `chased ${days}d ago`;
}

/** The draft as one pasteable block. The subject is labelled rather than merged
 *  into the body: a mail client keeps them in separate fields, and an unlabelled
 *  first line silently becomes part of the message. The recipient is prefixed only
 *  when linked mail supplied one — the commonest silent application has nobody to
 *  address, and an empty "To:" would read as a missing value rather than an absent one. */
export function clipboardText(draft: FollowUpDraft): string {
  const head = draft.recipient ? `To: ${draft.recipient}\n` : '';
  return `${head}Subject: ${draft.subject}\n\n${draft.body}`;
}
