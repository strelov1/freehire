import type { Job } from '../generated/contracts';
import type { TimelineEvent } from './experience';
import type { AutoApplyReviewInfo } from './tracking';

/** The classification/link overlay an inbox email carries: its classified status
 *  and, when resolved, the linked application (slug + company) or a pending
 *  suggestion the reading pane confirms inline. All optional/omitempty. */
export interface EmailLinking {
  status_signal?: string;
  link_source?: string;
  linked_slug?: string;
  linked_company?: string;
  suggested_slug?: string;
  suggested_company?: string;
}

/** One email linked to an application, on the application detail page. */
export interface ApplicationEmail {
  direction?: 'incoming' | 'outgoing';
  to_addrs?: string;
  cc_addrs?: string;
  id: number;
  source: string;
  from_addr: string;
  from_name: string;
  subject: string;
  status_signal?: string;
  link_source?: string;
  received_at: string;
  read: boolean;
}

/** One message the mailbox sweep proposes for an application. It is a SUGGESTION, not a
 *  link: it is resolved through the same confirm/reject calls the inbox uses. */
export interface RecalledEmail {
  /** Our stored row's id, and 0 for a message the mailbox search found — that one is not
   *  stored at all until you link it. */
  id: number;
  /** Names the message at the mail provider. Present on the search path, and what the link
   *  call is given. */
  provider_id?: string;
  from_addr: string;
  from_name: string;
  subject: string;
  received_at: string;
  /** This message carries a calendar invitation's identifier, so confirming it is what
   *  brings the meeting in. */
  invitation: boolean;
}

/** The result of one sweep. `scanned` is what makes an empty `suggested` legible: nothing
 *  found among forty messages is a different answer from nothing to look at. */
export interface MailRecallResult {
  scanned: number;
  suggested: RecalledEmail[];
  /** How many of the proposed messages carry an invitation identifier. The meetings
   *  themselves arrive on the next calendar sync. */
  invitations: number;
}

/** An offer to move the application to the stage its newest classified message implies.
 *  Present only when the two disagree and the candidate has not already answered — mail
 *  never settles an application by itself, and this is how that rule is stated rather
 *  than left to be inferred from a stage that did not move. */
export interface StageSuggestion {
  stage: string;
  signal: string;
  email_id: number;
}

/** The caller's application for one job slug, with the emails linked to it. */
export interface TrackedApplication {
  job: Job;
  viewed_at: string | null;
  saved_at: string | null;
  applied_at: string | null;
  stage?: string;
  notes?: string;
  /** When the caller last recorded chasing this application, or null for never. */
  followed_up_at: string | null;
  /** When somebody last opened a CV sent for this application. Outside the silence derivation:
   *  a recruiter reading a CV is not a reply. */
  cv_opened_at: string | null;
  emails: ApplicationEmail[];
  stage_suggestion?: StageSuggestion;
  /** The application's history from the ledger, newest first — what happened to it, as
   *  against the marks on the posting the listing row carries. Empty when nothing has. */
  events: TimelineEvent[];
  /** The caller's live auto-apply attempt for this job, absent when none exists
   *  (openspec/changes/auto-apply-review-tracking). The drawer's own approve/decline
   *  banner reads this. */
  auto_apply?: AutoApplyReviewInfo;
}

export interface FollowUpDraft {
  subject: string;
  body: string;
  recipient?: string;
  recipient_name?: string;
  days_silent: number;
}
