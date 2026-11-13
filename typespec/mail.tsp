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
  /** One value from the vocabulary above. (e.g. `interview_invitation`) */
  signal?: string | null;
  /** Link the message to this application at the same time. */
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

/** An offer to move the application to the stage its newest classified message implies. */
model StageSuggestion {
  stage: string;
  signal: string;
  email_id: int64;
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
  stage_suggestion?: StageSuggestion;

  /** The caller's live auto-apply attempt for this job; absent when none exists. */
  auto_apply?: AutoApplyReviewInfo;
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

/**
 * `available` reflects whether the instance has Gmail OAuth configured. `status` is
 * `needs_reconsent` when Google revoked the grant. Connecting itself is a browser
 * redirect (`/me/gmail/connect`), so it is session-only and not part of the key-driven
 * surface.
 */
@summary("Gmail connection status.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/gmail")
@get
op gmailStatus(): Data<GmailStatus> | Problem;

// Revokes the grant (best effort) and purges the token and the synced Gmail mail.
/**
 * Best-effort revokes the grant at Google, deletes the stored token, and removes the
 * Gmail-sourced mail. Mail from the hosted address stays.
 */
@summary("Disconnect Gmail and purge the mail it synced.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      connected: false,
    },
  },
})
@tag("Mail")
@route("/me/gmail")
@delete
op gmailDisconnect(): Data<GmailDisconnected> | Problem;

// Starts a background sync. Registered only where Gmail is configured.
/**
 * Returns as soon as the sync is queued — a full backfill outlives the request — so poll
 * the inbox for results rather than reading counts here.
 */
@summary("Start an incremental sync.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      started: true,
    },
  },
})
@tag("Mail")
@route("/me/gmail/sync")
@post
op syncGmail(): Data<GmailSyncStarted> | Problem;

// The caller's mail, newest first, without deleted messages. Filters: `source`,
// `status`, `link`, `q`, `unread`, `unclassified`, `include_other`; `body` adds each
// message's body (pages are then capped lower). A page param that does not read falls
// back to its default.
/**
 * `?body=1` returns each message's readable text inline and marks nothing read — that is
 * the agent read path, so sweeping a backlog never zeroes the owner's unread count
 * (`GET /me/emails/{id}` does mark read). Pages carrying bodies are capped at 50. An
 * unknown `source`, `status` or `link` value is a 400, never a silently empty page.
 */
@summary("List your mail, newest first, excluding deleted.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/inbox")
@get
op getInbox(
  /** One account: `gmail`, `hosted`, or `external`. */
  @query source?: string,
  /** One classified label (see the triage vocabulary below). (e.g. `rejection`) */
  @query status?: string,
  /** One link state: `linked`, `suggested` (awaiting your word), or `unlinked`. */
  @query link?: string,
  /** Match subject, sender, or body. */
  @query q?: string,
  /** Only messages you have not opened. */
  @query unread?: string,
  /** Only mail nothing has judged yet — the agent's work queue. */
  @query unclassified?: string,
  /**
   * Also list the `other` label the default hides. Asking for it by `status=other`
   * implies this.
   */
  @query include_other?: string,
  /** Include each message body; caps the page at 50. */
  @query body?: string,
  /** Page size. */
  @query limit?: string,
  /** Page offset. */
  @query offset?: string,
): InboxPage | Problem;

// Marks every unread message matching the same filters as the listing.
/**
 * It takes the same filters as `GET /me/inbox` and marks exactly what that listing shows
 * — including the default that hides the `other` label, so an unfiltered call leaves that
 * mail unread. There is no undo.
 */
@summary("Mark every unread message matching the filters as read.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      marked: 12,
    },
  },
})
@tag("Mail")
@route("/me/inbox/read-all")
@post
op markAllReadInbox(
  /** Narrow to one account. */
  @query source?: string,
  /** Narrow to one label. */
  @query status?: string,
  /** Narrow to one link state. */
  @query link?: string,
  /** Narrow by search. */
  @query q?: string,
  @query unread?: string,
  /** Narrow to mail nothing has judged yet. */
  @query unclassified?: string,
  /** Also mark the `other` label the default hides. */
  @query include_other?: string,
): Data<InboxMarked> | Problem;

// Opening a message marks it read. 404 for another user's, or an id that is not a
// number.
/**
 * Use `GET /me/inbox?body=1` instead when sweeping a backlog — `read_at` means "a human
 * saw this", and this endpoint sets it.
 */
@summary("One message in full. Marks it read.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}")
@get
op getEmail(/** The message id. (e.g. `4821`) */ @path id: string): Data<InboxEmail> | Problem;

/**
 * A later re-sync will not resurrect it — deletion is reader state, not mail-server
 * state.
 */
@summary("Soft-delete a message. Answers 204.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/delete")
@post
op deleteEmail(/** The message id. */ @path id: string): NoContentResponse | Problem;

@summary("Undo a soft-delete. Answers 204.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/restore")
@post
op restoreEmail(/** The message id. */ @path id: string): NoContentResponse | Problem;

// A batch of messages the caller's own mail client fetched, as `messages`: a list of
// objects with `external_id`, `thread_id`, `from_addr`, `from_name`, `subject`,
// `body_text`, `body_html` and `received_at`. The mail service validates the batch and
// refuses it whole with a 400.
/**
 * Stored under source `external`. freehire provides no transport here and never
 * classifies this mail — that is the point of the tier. `external_id` (the Message-ID in
 * practice) is the dedup key: re-pushing the same id updates that message instead of
 * storing a copy, so a nightly re-sync is safe. Content columns are refreshed; your read,
 * deleted and triage state is not, so a re-sync cannot resurrect deleted mail or wipe a
 * verdict. At most 100 messages per call, applied as one transaction — an oversized batch
 * is refused, never truncated.
 */
@summary("Push a batch your own mail client fetched.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      messages: #[
        #{
          external_id: "<CAF...@mail.gmail.com>",
          from_addr: "no-reply@us.greenhouse-mail.io",
          subject: "Thank you for applying",
          received_at: utcDateTime.fromISO("2026-07-24T09:12:00Z"),
        },
      ],
    },
  },
  returnType: #{
    data: #{
      inserted: 1,
      updated: 0,
    },
  },
})
@tag("Mail")
@route("/me/emails")
@post
op ingestEmails(@body body: unknown): Data<InboxIngested> | Problem;

/**
 * The vocabulary is closed: `acknowledgement`, `screening`, `interview_invitation`,
 * `assessment`, `offer`, `rejection`, `info_request`, `incomplete_application`, `other`.
 * Anything else is refused. A confident verdict on a linked message advances the
 * application's stage — strictly forward, never out of a settled stage, and a rejection
 * never advances anything by itself.
 */
@summary("Record what a message is, and optionally which application it belongs to.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/triage")
@post
op triageEmail(/** The message id. */ @path id: string, @body body: InboxTriage): Data<InboxEmail> | Problem;

@summary("Attach a message to one of your applications.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/link")
@post
op linkEmail(/** The message id. */ @path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

@summary("Clear a message's application link; its classification stays.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/unlink")
@post
op unlinkEmail(/** The message id. */ @path id: string): Data<InboxEmail> | Problem;

/**
 * Only a deterministic match — the mail thread, or the company name in the sender or
 * subject — links itself. A model's pick lands here as a suggestion instead, and
 * `?link=suggested` is the queue of them.
 */
@summary("Accept a suggested link.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/confirm")
@post
op confirmEmailLink(/** The message id. */ @path id: string): Data<InboxEmail> | Problem;

@summary("Dismiss a suggested link without attaching it.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/reject")
@post
op rejectEmailLink(/** The message id. */ @path id: string): Data<InboxEmail> | Problem;

// Records the application from the email and links it, dated by the email. 409 while a
// suggestion is pending.
/**
 * The way out of `?link=unlinked`: mail about a job you never tracked. The application is
 * dated by the mail's `received_at`, not by now — the application demonstrably existed by
 * the time the employer wrote.
 */
@summary("Record an application from a message, and link the message to it.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/emails/{id}/application")
@post
op createApplicationFromEmail(/** The message id. */ @path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

// The caller's application for a job slug with its linked mail. 404 when not tracked.
/**
 * `events` is the application's ledger, newest first — the apply, employer replies,
 * follow-ups, stage changes, scheduled interviews — in the shape `GET /me/timeline`
 * serves, bounded at 100 and empty when nothing has happened yet. This read also carries
 * the full job view; the listing carries only a card. A slug you do not track is a 404.
 */
@summary("One tracked application, with the mail linked to it and its history.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/tracking/{slug}")
@get
op getTrackedApplication(/** The job `public_slug`. */ @path slug: string): Data<InboxApplicationDetail> | Problem;

// 409 unless the application is waiting on a reply.
/**
 * Offered only when the application is in the silent state the tracking board’s own badge
 * uses — a `409` otherwise. `recipient`/`recipient_name` are omitted when nobody at the
 * company ever replied, which is the common case: the draft is handed to you, nothing is
 * sent.
 */
@summary("Draft a chase for a silent application.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      subject: "Following up: Senior Go Engineer application",
      body: "Hi …",
      recipient: "recruiting@acme.com",
      recipient_name: "Acme Recruiting",
      days_silent: 12,
    },
  },
})
@tag("Mail")
@route("/me/tracking/{slug}/followup")
@get
op getApplicationFollowUp(/** The job `public_slug`. */ @path slug: string): Data<MailFollowUpDraft> | Problem;

/**
 * Does not itself affect the silence calculation — that reads when the other side last
 * moved, and a chase is not a reply. Idempotent within the hour: a double click
 * overwrites the timestamp rather than logging a second event. Returns `204 No Content`.
 */
@summary("Record that you sent a chase.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/tracking/{slug}/followup")
@post
op recordApplicationFollowUp(/** The job `public_slug`. */ @path slug: string): NoContentResponse | Problem;

// Sweeps the mailbox for mail belonging to the application and records suggestions.
// Paid AI.
/**
 * For an application whose mail arrived before you connected an inbox, or that a sync
 * missed. Runs an LLM pass over your mailbox and returns matches as suggestions — it
 * never links or writes the ledger itself; confirm a pick with
 * `POST /me/tracking/{slug}/mail-recall/link`. A `503` when no mail gateway is
 * configured, a `502` when the model call fails.
 */
@summary("Sweep your connected mailbox for mail belonging to this application.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      scanned: 340,
      suggested: #[
        #{
          id: 0,
          provider_id: "18f2a…",
          from_addr: "recruiting@acme.com",
          from_name: "Acme Recruiting",
          subject: "Interview invitation",
          received_at: utcDateTime.fromISO("2026-07-15T09:00:00Z"),
          invitation: true,
        },
      ],
      invitations: 1,
    },
  },
})
@tag("Mail")
@route("/me/tracking/{slug}/mail-recall")
@post
op recallApplicationMail(/** The job `public_slug`. */ @path slug: string): Data<MailRecallResult> | Problem;

// Imports a message the sweep proposed and links it to the application.
/**
 * Imports the message from your mailbox by `provider_id` (from a mail-recall response)
 * and links it to the application in one step — the same import-then-link path a Gmail
 * sync uses, so the response is the full message body, the same shape
 * `POST /me/emails/{id}/link` returns.
 */
@summary("Import and link one message a mail-recall sweep proposed.")
@useAuth(SessionOrKey)
@tag("Mail")
@route("/me/tracking/{slug}/mail-recall/link")
@post
op linkRecalledMail(/** The job `public_slug`. */ @path slug: string, @body body?: MailRecalledPick): Data<InboxEmail> | Problem;

/**
 * `address` is null until you claim one; `available` is false when the instance runs
 * without a mailbox domain.
 */
@summary("Your hosted freehire address, and whether the feature is configured.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      available: true,
      address: "you@mail.freehire.me",
    },
  },
})
@tag("Mail")
@route("/me/mailbox")
@get
op getMailbox(): Data<MailboxStatus> | Problem;

/** Registered only where a receiving domain is configured. */
@summary("Claim your hosted address.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      available: true,
      address: "you@mail.freehire.me",
    },
  },
})
@tag("Mail")
@route("/me/mailbox")
@post
op claimMailbox(): Data<MailboxStatus> | Problem;

/** Drops the mailbox and its received mail; Gmail mail stays. */
@summary("Release the address; it stops receiving.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      available: true,
      address: null,
    },
  },
})
@tag("Mail")
@route("/me/mailbox")
@delete
op releaseMailbox(): Data<MailboxStatus> | Problem;
