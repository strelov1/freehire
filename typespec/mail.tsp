import "@typespec/http";

using Http;

namespace Freehire;

// The caller's mail: the Gmail connection, the unified inbox, the link between a message
// and an application, the follow-up draft, the mail recall sweep and the hosted mailbox.
// With a session or a full-scope API key. The OAuth redirects (Gmail and calendar connect
// and callback) stay hand-written: they answer with redirects, not JSON.

/**
 * Whether Gmail is connected. `email` and `status` are sent only when a connection row
 * exists; `available` says whether the connect flow is configured on this deployment.
 */
model GmailStatus {
  connected: boolean;
  email?: string;
  status?: string;
  available: boolean;
  calendar_connected: boolean;
  mentor_calendar_connected: boolean;
  mentor_busy_sync_connected: boolean;
}

model GmailDisconnected {
  connected: boolean;
}

model GmailSyncStarted {
  started: boolean;
}

/** The classification and link overlay every message carries; each is absent when empty. */
model InboxLinking {
  status_signal?: string;
  link_source?: string;
  linked_slug?: string;
  linked_company?: string;
  suggested_slug?: string;
  suggested_company?: string;
}

/** One row of the listing. `body_text` is sent only when the listing asked for bodies. */
model InboxMessage {
  id: int64;
  source: string;
  external_id: string;
  from_addr: string;
  from_name: string;
  subject: string;
  snippet: string;
  body_text?: string;
  received_at: utcDateTime;
  read: boolean;
  ...InboxLinking;
}

/** One message with its body. The internal raw-MIME pointer is not exposed. */
model InboxEmail {
  id: int64;
  source: string;
  external_id: string;
  from_addr: string;
  from_name: string;
  subject: string;
  body_text: string;
  body_html: string;
  received_at: utcDateTime;
  read: boolean;
  ...InboxLinking;
}

model InboxMeta {
  ...PageMeta;

  /** How many messages the default filter suppressed. */
  hidden: int64;
}

model InboxPage {
  data: InboxMessage[];
  meta: InboxMeta;
}

model InboxMarked {
  marked: int64;
}

/** The verdict an agent records for one message. Left-out fields read as empty. */
model InboxTriage {
  signal?: string | null;
  slug?: string | null;

  /** Stored for display; gates nothing. */
  confidence?: float32 | null;
}

/** Names the application: a catalogue job's slug. Required, 400 when empty. */
model InboxLinkTarget {
  slug?: string | null;
}

model InboxIngested {
  inserted: int64;
  updated: int64;
}

model InboxApplicationEmail {
  id: int64;
  source: string;
  from_addr: string;
  from_name: string;
  subject: string;
  status_signal?: string;
  link_source?: string;
  received_at: utcDateTime;
  read: boolean;
}

/** The job, the caller's interaction with it, the linked mail and the history. */
model InboxApplicationDetail {
  /** The job in the shared wire shape. */
  job: unknown;

  viewed_at: utcDateTime | null;
  saved_at: utcDateTime | null;
  applied_at: utcDateTime | null;
  stage?: string;
  notes?: string;
  followed_up_at: utcDateTime | null;
  emails: InboxApplicationEmail[];

  /** The ledger, newest first. */
  events: TimelineEvent[];

  /** The stage the newest classified message implies; absent when there is nothing to offer. */
  stage_suggestion?: unknown;

  /** The caller's live auto-apply attempt for this job; absent when none exists. */
  auto_apply?: unknown;
}

/** A chase the candidate sends themselves; `recipient` is absent when nobody ever replied. */
model MailFollowUpDraft {
  subject: string;
  body: string;
  recipient?: string;
  recipient_name?: string;
  days_silent: int64;
}

/** One message the recall sweep proposes. `id` is 0 for a message the search found. */
model MailRecalledEmail {
  id: int64;
  provider_id?: string;
  from_addr: string;
  from_name: string;
  subject: string;
  received_at: utcDateTime;
  invitation: boolean;
}

model MailRecallResult {
  scanned: int64;
  suggested: MailRecalledEmail[];
  invitations: int64;
}

/** The message the caller picked out of a sweep. Required, 400 when empty. */
model MailRecalledPick {
  provider_id?: string | null;
}

model MailboxStatus {
  available: boolean;
  address: string | null;
}

@tag("Mail")
@route("/me/gmail")
@get
op gmailStatus(): Data<GmailStatus> | Problem;

/** Revokes the grant (best effort) and purges the token and the synced Gmail mail. */
@tag("Mail")
@route("/me/gmail")
@delete
op gmailDisconnect(): Data<GmailDisconnected> | Problem;

/** Starts a background sync. Registered only where Gmail is configured. */
@tag("Mail")
@route("/me/gmail/sync")
@post
op syncGmail(): Data<GmailSyncStarted> | Problem;

/**
 * The caller's mail, newest first, without deleted messages. Filters: `source`, `status`,
 * `link`, `q`, `unread`, `unclassified`, `include_other`; `body` adds each message's body
 * (pages are then capped lower). A page param that does not read falls back to its default.
 */
@tag("Mail")
@route("/me/inbox")
@get
op getInbox(
  @query source?: string,
  @query status?: string,
  @query link?: string,
  @query q?: string,
  @query unread?: string,
  @query unclassified?: string,
  @query include_other?: string,
  @query body?: string,
  @query limit?: string,
  @query offset?: string,
): InboxPage | Problem;

/** Marks every unread message matching the same filters as the listing. */
@tag("Mail")
@route("/me/inbox/read-all")
@post
op markAllReadInbox(
  @query source?: string,
  @query status?: string,
  @query link?: string,
  @query q?: string,
  @query unread?: string,
  @query unclassified?: string,
  @query include_other?: string,
): Data<InboxMarked> | Problem;

/** Opening a message marks it read. 404 for another user's, or an id that is not a number. */
@tag("Mail")
@route("/me/emails/{id}")
@get
op getEmail(@path id: string): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/emails/{id}/delete")
@post
op deleteEmail(@path id: string): NoContentResponse | Problem;

@tag("Mail")
@route("/me/emails/{id}/restore")
@post
op restoreEmail(@path id: string): NoContentResponse | Problem;

/**
 * A batch of messages the caller's own mail client fetched, as `messages`: a list of
 * objects with `external_id`, `thread_id`, `from_addr`, `from_name`, `subject`, `body_text`,
 * `body_html` and `received_at`. The mail service validates the batch and refuses it whole with a 400.
 */
@tag("Mail")
@route("/me/emails")
@post
op ingestEmails(@body body: unknown): Data<InboxIngested> | Problem;

@tag("Mail")
@route("/me/emails/{id}/triage")
@post
op triageEmail(@path id: string, @body body: InboxTriage): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/emails/{id}/link")
@post
op linkEmail(@path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/emails/{id}/unlink")
@post
op unlinkEmail(@path id: string): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/emails/{id}/confirm")
@post
op confirmEmailLink(@path id: string): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/emails/{id}/reject")
@post
op rejectEmailLink(@path id: string): Data<InboxEmail> | Problem;

/** Records the application from the email and links it, dated by the email. 409 while a suggestion is pending. */
@tag("Mail")
@route("/me/emails/{id}/application")
@post
op createApplicationFromEmail(@path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

/** The caller's application for a job slug with its linked mail. 404 when not tracked. */
@tag("Mail")
@route("/me/tracking/{slug}")
@get
op getTrackedApplication(@path slug: string): Data<InboxApplicationDetail> | Problem;

/** 409 unless the application is waiting on a reply. */
@tag("Mail")
@route("/me/tracking/{slug}/followup")
@get
op getApplicationFollowUp(@path slug: string): Data<MailFollowUpDraft> | Problem;

@tag("Mail")
@route("/me/tracking/{slug}/followup")
@post
op recordApplicationFollowUp(@path slug: string): NoContentResponse | Problem;

/** Sweeps the mailbox for mail belonging to the application and records suggestions. Paid AI. */
@tag("Mail")
@route("/me/tracking/{slug}/mail-recall")
@post
op recallApplicationMail(@path slug: string): Data<MailRecallResult> | Problem;

/** Imports a message the sweep proposed and links it to the application. */
@tag("Mail")
@route("/me/tracking/{slug}/mail-recall/link")
@post
op linkRecalledMail(@path slug: string, @body body?: MailRecalledPick): Data<InboxEmail> | Problem;

@tag("Mail")
@route("/me/mailbox")
@get
op getMailbox(): Data<MailboxStatus> | Problem;

/** Registered only where a receiving domain is configured. */
@tag("Mail")
@route("/me/mailbox")
@post
op claimMailbox(): Data<MailboxStatus> | Problem;

/** Drops the mailbox and its received mail; Gmail mail stays. */
@tag("Mail")
@route("/me/mailbox")
@delete
op releaseMailbox(): Data<MailboxStatus> | Problem;
