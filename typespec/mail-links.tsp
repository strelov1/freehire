import "@typespec/http";

using Http;

namespace Freehire;

// The caller's mail, continued from mail.tsp: triaging a message, linking it to an
// application or unlinking it, confirming or rejecting a suggested link, recording an
// application from a message, and the AI reply draft.

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
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
        status_signal: "interview_invitation",
        linked_slug: "tech-lead-web-core-speechify-a1b2",
      },
    },
})
@route("/me/emails/{id}/triage")
@post
op triageEmail(/** The message id. */ @path id: string, @body body: InboxTriage): Data<InboxEmail> | Problem;

@summary("Attach a message to one of your applications.")
@useAuth(SessionOrKey)
@tag("Mail")
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
        link_source: "manual",
        linked_slug: "tech-lead-web-core-speechify-a1b2",
      },
    },
})
@route("/me/emails/{id}/link")
@post
op linkEmail(/** The message id. */ @path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

@summary("Clear a message's application link; its classification stays.")
@useAuth(SessionOrKey)
@tag("Mail")
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
        status_signal: "acknowledgement",
      },
    },
})
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
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
        link_source: "manual",
        linked_slug: "tech-lead-web-core-speechify-a1b2",
      },
    },
})
@route("/me/emails/{id}/confirm")
@post
op confirmEmailLink(/** The message id. */ @path id: string): Data<InboxEmail> | Problem;

@summary("Dismiss a suggested link without attaching it.")
@useAuth(SessionOrKey)
@tag("Mail")
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
      },
    },
})
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
@opExample(#{
  returnType: #{
      data: #{
        id: 4821,
        source: "gmail",
        external_id: "18c2f4a1b3d5e6f7",
        from_addr: "no-reply@us.greenhouse-mail.io",
        from_name: "Acme Recruiting",
        subject: "Thank you for applying to Acme",
        body_text: "Dear candidate, thank you for applying.",
        body_html: "<p>Dear candidate, thank you for applying.</p>",
        received_at: utcDateTime.fromISO("2026-06-18T09:12:00Z"),
        read: true,
        linked_slug: "tech-lead-web-core-speechify-a1b2",
      },
    },
})
@route("/me/emails/{id}/application")
@post
op createApplicationFromEmail(/** The message id. */ @path id: string, @body body?: InboxLinkTarget): Data<InboxEmail> | Problem;

/** A reply for the candidate to review and send from their own mail client. */
model MailReplyDraft {
  /** The address the received message came from. */
  recipient: string;
  recipient_name: string;
  subject: string;
  body: string;
}

/**
 * One model call on the caller's daily `mail-reply` allowance; every request is a new
 * draft and is charged again. Nothing is sent or stored: the candidate edits the text and
 * sends it themselves. Facts only the candidate knows (availability, salary) come back as
 * bracketed placeholders. 409 for a message the caller sent; 402 without a paid plan or
 * when the allowance is spent; 502 when the model fails (the charge is returned).
 */
@summary("Draft a reply to a received message.")
@useAuth(SessionOrKey)
@tag("Mail")
@opExample(#{
  returnType: #{
    data: #{
      recipient: "anna@acme.io",
      recipient_name: "Anna Lee",
      subject: "Re: Interview next week",
      body: "Hi Anna,\n\nThank you for the invitation. I would be glad to talk; [times that suit you] work for me.\n\nBest regards,\n[Your name]",
    },
  },
})
@route("/me/emails/{id}/reply-draft")
@post
op draftEmailReply(/** The message id. */ @path id: string):
  | Data<MailReplyDraft>
  | {
      @statusCode _: 402;
      @body body: CvPlanRefusal;
    }
  | Problem;
