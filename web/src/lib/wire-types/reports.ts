import type { components } from '#lib/generated/api.js';

/** Why a job was reported. A closed vocabulary mirroring the backend's
 *  internal/report reasons; labels live in #lib/reports. */
export type ReportReason = components['schemas']['JobReportReason'];

/** What a candidate can report about how a company hires. A closed vocabulary
 *  mirroring the backend's vocab.CompanyProcessReportKindValues and the CHECK in
 *  migration 0156. Each value is a statement that is either true of an employer or
 *  not, which is why one reporter is enough to show it and why nothing here reaches
 *  a moderator. */
export type ProcessReportKind = components['schemas']['ProcessReportKind'];

/** Everything the report picker can offer. It is deliberately WIDER than
 *  ReportReason: the picker is one list to a person, but its entries land on two
 *  different endpoints, and only ReportReason's values are ones the moderation
 *  endpoint will accept. Keeping them separate types is what stops an entry drifting
 *  onto the route that would file it as a ticket for a reviewer who cannot act on it. */
export type ReportPickerValue = ReportReason | ProcessReportKind;

/** What a process-report write answers with: the kind acted on and the company's
 *  resulting count. The count comes back on every write so a caller renders the badge
 *  from this response rather than re-reading the company. */
export type ProcessReportResult = components['schemas']['ProcessReport'];

/** A user's report of a problem with a live vacancy. `reporter_email` and the
 *  `job_*` fields are present only on the moderator review queue, never on the
 *  reporter's own create response (which also never carries the reporter id). `notified`
 *  is present only on a decision response: whether the reporter was actually emailed. A
 *  decision always stands, so `false` means "recorded, nobody told". */
export type Report = components['schemas']['JobReport'];

/** The content a user submits when reporting a job. */
export interface ReportInput {
  reason: ReportReason;
  /** Optional elaboration. The reason vocabulary already says what is wrong. */
  details?: string;
  contact_telegram?: string;
}

/** A ghost-signal claim: the caller applied to a job on `applied_on` and was
 *  never answered. Distinct from a moderation Report — it accumulates as
 *  evidence rather than reaching a moderator, and can be withdrawn. */
export interface GhostReportInput {
  /** Plain calendar date (YYYY-MM-DD): a day, not an instant. */
  applied_on: string;
}
