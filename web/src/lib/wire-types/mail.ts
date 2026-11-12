import type { components } from '#lib/generated/api.js';
import type { Job } from '../generated/contracts';

/** The classification/link overlay an inbox email carries: its classified status
 *  and, when resolved, the linked application (slug + company) or a pending
 *  suggestion the reading pane confirms inline. All optional/omitempty. */
export type EmailLinking = components['schemas']['InboxLinking'];

/** One email linked to an application, on the application detail page. */
export type ApplicationEmail = components['schemas']['InboxApplicationEmail'];

/** One message the mailbox sweep proposes for an application. It is a SUGGESTION, not a
 *  link: it is resolved through the same confirm/reject calls the inbox uses. */
export type RecalledEmail = components['schemas']['MailRecalledEmail'];

/** The result of one sweep. `scanned` is what makes an empty `suggested` legible: nothing
 *  found among forty messages is a different answer from nothing to look at. */
export type MailRecallResult = components['schemas']['MailRecallResult'];

/** An offer to move the application to the stage its newest classified message implies.
 *  Present only when the two disagree and the candidate has not already answered — mail
 *  never settles an application by itself, and this is how that rule is stated rather
 *  than left to be inferred from a stage that did not move. */
export type StageSuggestion = components['schemas']['StageSuggestion'];

/** The caller's application for one job slug, with the emails linked to it. */
export type TrackedApplication = Omit<components['schemas']['InboxApplicationDetail'], 'job'> & {
  /** The job in the shared wire shape; raw in the contract, whose Job type is generated from Go. */
  job: Job;
};

export type FollowUpDraft = components['schemas']['MailFollowUpDraft'];
