import type { Http } from '../api-core';
import type { FollowUpDraft, MailRecallResult, TrackedApplication } from '../types';
import type { EmailBody, GmailStatus, InboxMessage, InboxSource, MailboxStatus, Page, Slice } from '../api-types';
import { jsonBody, toSlice } from '../api-core';

export function mailApi({ call, request, requestData }: Http) {
  // --- Gmail inbox ---------------------------------------------------------

  /** Whether the caller has connected Gmail for the ATS inbox. */
  async function gmailStatus(): Promise<GmailStatus> {
    return requestData<GmailStatus>('/api/v1/me/gmail');
  }

  /** Disconnect Gmail: revoke the grant and purge synced mail. */
  async function disconnectGmail(): Promise<void> {
    await requestData<unknown>('/api/v1/me/gmail', { method: 'DELETE' });
  }

  /** Trigger an on-demand sync of the caller's ATS mail (runs in the background). */
  async function syncGmail(): Promise<void> {
    await requestData<unknown>('/api/v1/me/gmail/sync', { method: 'POST' });
  }

  /** A page of the flat inbox listing, newest first. Optional search term filters
   *  by subject, sender, or body; optional source narrows to one account (the
   *  switcher). `meta.total` reflects the filtered count, so the Paginator pages
   *  over the matches. */
  async function getInbox(
    opts: {
      q?: string;
      limit?: number;
      offset?: number;
      source?: InboxSource;
      unread?: boolean;
      status?: string;
      /** Ask for mail the classifier judged not to be about an application at all. The
       *  listing hides it by default and reports how many it hid. */
      includeOther?: boolean;
    } = {},
  ): Promise<Slice<InboxMessage> & { hidden: number }> {
    const params = new URLSearchParams({
      limit: String(opts.limit ?? 20),
      offset: String(opts.offset ?? 0),
    });
    if (opts.q) params.set('q', opts.q);
    if (opts.source) params.set('source', opts.source);
    if (opts.unread) params.set('unread', '1');
    if (opts.status) params.set('status', opts.status);
    if (opts.includeOther) params.set('include_other', '1');
    const page = await request<Page<InboxMessage> & { meta: { hidden?: number } }>(
      `/api/v1/me/inbox?${params.toString()}`,
    );
    return { ...toSlice(page, opts.offset ?? 0), hidden: page.meta.hidden ?? 0 };
  }

  /** Mark every unread message matching the active filters as read; returns the
   *  count marked. The unread flag is implicit server-side, so it is not sent. */
  async function markAllRead(source: InboxSource = '', status = '', q = ''): Promise<number> {
    const params = new URLSearchParams();
    if (source) params.set('source', source);
    if (status) params.set('status', status);
    if (q) params.set('q', q);
    const res = await requestData<{ marked: number }>(`/api/v1/me/inbox/read-all?${params.toString()}`, {
      method: 'POST',
    });
    return res.marked;
  }

  /** Soft-delete a message (hidden from the inbox, restorable via restoreEmail). */
  async function deleteEmail(id: number): Promise<void> {
    await call(`/api/v1/me/emails/${id}/delete`, { method: 'POST' });
  }

  /** Undo a soft-delete, bringing the message back into the inbox. */
  async function restoreEmail(id: number): Promise<void> {
    await call(`/api/v1/me/emails/${id}/restore`, { method: 'POST' });
  }

  /** The caller's hosted-mailbox address (or null) + feature availability. */
  async function mailboxStatus(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox');
  }

  /** Claim (or return) the caller's hosted mailbox address. */
  async function claimMailbox(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox', { method: 'POST' });
  }

  /** Release the hosted mailbox: drop the address and purge its received mail. */
  async function releaseMailbox(): Promise<MailboxStatus> {
    return requestData<MailboxStatus>('/api/v1/me/mailbox', { method: 'DELETE' });
  }

  /** One message's full body. */
  async function getEmail(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}`);
  }

  /** The caller's application for a job slug, with its linked emails. */
  async function getTrackedApplication(slug: string): Promise<TrackedApplication> {
    return requestData<TrackedApplication>(`/api/v1/me/tracking/${encodeURIComponent(slug)}`);
  }

  /** Sweep the caller's mailbox for mail belonging to this application. The matches come
   *  back as SUGGESTIONS — nothing is linked — and are resolved with confirmEmailLink /
   *  rejectEmailLink, the same calls the inbox uses. 502 when the model could not be
   *  reached, which is deliberately not an empty result: "your mailbox holds nothing" is
   *  the wrong thing to say about a gateway being down. */
  async function recallApplicationMail(slug: string): Promise<MailRecallResult> {
    return requestData<MailRecallResult>(`/api/v1/me/tracking/${encodeURIComponent(slug)}/mail-recall`, {
      method: 'POST',
    });
  }

  /** Import a message the sweep found in the mailbox and link it to the application.
   *  The sweep itself stores nothing — a proposal lives on screen only — so this is the
   *  call that makes the message ours, and it is idempotent: a message the mail sync had
   *  already fetched is linked rather than copied. */
  async function linkRecalledMail(slug: string, providerId: string): Promise<EmailBody> {
    return requestData<EmailBody>(
      `/api/v1/me/tracking/${encodeURIComponent(slug)}/mail-recall/link`,
      jsonBody('POST', { provider_id: providerId }),
    );
  }

  /** The assembled follow-up draft for a silent application. 409 when the
   *  application is not waiting on a reply — the same verdict the board's badge
   *  renders, so a card that offers the draft never gets one. */
  async function getFollowUpDraft(slug: string): Promise<FollowUpDraft> {
    return requestData<FollowUpDraft>(`/api/v1/me/tracking/${encodeURIComponent(slug)}/followup`);
  }

  /** Record that the caller chased this application. Sends nothing — it stamps
   *  followed_up_at, which stays outside the silence derivation. 204, no body. */
  async function recordFollowUp(slug: string): Promise<void> {
    await call(`/api/v1/me/tracking/${encodeURIComponent(slug)}/followup`, { method: 'POST' });
  }

  /** Promote an email's pending suggestion to a confirmed link. */
  async function confirmEmailLink(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/confirm`, { method: 'POST' });
  }

  /** Dismiss an email's pending suggestion without linking. */
  async function rejectEmailLink(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/reject`, { method: 'POST' });
  }

  /** Manually link an email to the application named by slug. */
  async function linkEmail(id: number, slug: string): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/link`, jsonBody('POST', { slug }));
  }

  /** Clear an email's application link. */
  async function unlinkEmail(id: number): Promise<EmailBody> {
    return requestData<EmailBody>(`/api/v1/me/emails/${id}/unlink`, { method: 'POST' });
  }

  return {
    gmailStatus,
    disconnectGmail,
    syncGmail,
    getInbox,
    markAllRead,
    deleteEmail,
    restoreEmail,
    mailboxStatus,
    claimMailbox,
    releaseMailbox,
    getEmail,
    getTrackedApplication,
    recallApplicationMail,
    linkRecalledMail,
    getFollowUpDraft,
    recordFollowUp,
    confirmEmailLink,
    rejectEmailLink,
    linkEmail,
    unlinkEmail,
  };
}
