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

/** How the card reports that somebody opened a CV sent for this application, or null when none
 *  was, or when the timestamp will not parse.
 *
 *  A third reading beside the silence badge and the chase, and like them it neither replaces nor
 *  softens them: `cv_opened_at` is outside the server's last-activity derivation on purpose (see
 *  migration 0060), because a recruiter reading a CV is not a reply. "24d" and "CV opened 2d ago"
 *  on one card is the useful pair — they read it and still said nothing.
 *
 *  Whole days, floored at zero, matching the unit of the badge beside it. */
export function cvOpenedLabel(item: MyJob, now: Date = new Date()): string | null {
  if (!item.cv_opened_at) return null;
  const at = new Date(item.cv_opened_at);
  if (Number.isNaN(at.getTime())) return null;
  const days = Math.floor((now.getTime() - at.getTime()) / 86_400_000);
  if (days <= 0) return 'CV opened today';
  if (days === 1) return 'CV opened yesterday';
  return `CV opened ${days}d ago`;
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

// Both compose links open a window in the candidate's own client and never send —
// the same handoff the copy action makes, one click shorter. encodeURIComponent turns
// the paragraph breaks into %0A; left raw, some clients run the paragraphs together.
// Both targets cap the URL near 2 KB, comfortably above a draft this size, and the
// copy action stays as the escape hatch if that ever stops being true.

/** A `mailto:` for the candidate's default mail client. An unaddressed draft still
 *  opens a compose window with the To field left blank — that is the commonest
 *  silent application, and withholding the link there would help nobody. */
export function mailtoHref(draft: FollowUpDraft): string {
  const to = draft.recipient ? mailtoAddress(draft.recipient) : '';
  return `mailto:${to}?subject=${encodeURIComponent(draft.subject)}&body=${encodeURIComponent(draft.body)}`;
}

/** RFC 6068 keeps the `@` literal in a mailto addr-spec, and clients that do not decode
 *  `%40` open with an empty To field — indistinguishable, to the candidate, from the link
 *  being broken. Everything either side of it is still encoded: the address comes from a
 *  parsed mail header, so a stray `?` or `&` would otherwise inject mailto parameters. */
function mailtoAddress(addr: string): string {
  const at = addr.lastIndexOf('@');
  if (at < 0) return encodeURIComponent(addr);
  return `${encodeURIComponent(addr.slice(0, at))}@${encodeURIComponent(addr.slice(at + 1))}`;
}

/** Gmail's web compose (`su`, not `subject`). Worth offering beside `mailto:` because
 *  a browser whose OS registers no mail handler does nothing at all on a mailto click
 *  — a silent failure the candidate would read as the feature being broken. */
export function gmailHref(draft: FollowUpDraft): string {
  const to = draft.recipient ? `to=${encodeURIComponent(draft.recipient)}&` : '';
  return `https://mail.google.com/mail/?view=cm&fs=1&${to}su=${encodeURIComponent(draft.subject)}&body=${encodeURIComponent(draft.body)}`;
}
