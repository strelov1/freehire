# Interview scheduling

The opt-in agent negotiates interview times in the candidate's Gmail. The Gmail
sync and classifier establish the initial invitation and deterministic application
link. The worker reads subsequent messages from the live provider thread, verifies
the original recruiter and checks that the candidate has not answered it.

- Start only after explicit OAuth consent to Gmail read and send plus Calendar read
  and write. Each model call uses the candidate's paid entitlement, mail allowance
  and gateway attribution. The worker and connect flow share the rollout switch.
- The model classifies an untrusted message and chooses among approved outbound
  copy. Match evidence back to the message. All offered times come from the booking
  link service. Review ambiguous requests, other questions and automated senders.
- A durable reply attempt is recorded before the Gmail write. If the result is
  uncertain, check the original thread for its RFC Message-ID. Hand off when the
  send cannot be confirmed. Never blindly retry the send.
- Only confirm an interview in Gmail after Calendar has added the attendee to a
  Meet event and the booking records the provider event ID and Meet URL.
- A link booking by the same recruiter stops reminders. A silent recruiter gets
  at most two follow-ups. The daily inbox digest retires covered scheduling activity
  on confirmed delivery.

The production pilot uses two controlled mailboxes, with the candidate signed in
to Freehire and opted in through Integrations. Send from the recruiter mailbox to
the candidate, wait for Gmail sync and classification, and reply in the same Gmail
thread choosing a numbered option. Verify the reply, the candidate's Calendar,
the invitation in the recruiter's inbox, and the candidate's daily digest.
