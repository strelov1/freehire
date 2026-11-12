import type { Http } from '../api-core';
import type {
  Contribution,
  DiscordStatus,
  FoundJob,
  PrefillResult,
  ResolvedLink,
  Submission,
  SubmissionInput,
  Subscription,
  TelegramStatus,
} from '../types';
import type { components } from '#lib/generated/api.js';
import { jsonBody } from '../api-core';

export function submissionsApi({ call, request, requestData }: Http) {
  /** The caller's notification subscriptions (one per saved search + channel). */
  async function listSubscriptions(): Promise<Subscription[]> {
    return requestData<Subscription[]>('/api/v1/me/subscriptions');
  }

  /** Subscribe a saved search to a channel (telegram by default). A duplicate is a 409. */
  async function createSubscription(savedSearchId: number, channel = 'telegram'): Promise<Subscription> {
    return requestData<Subscription>(
      '/api/v1/me/subscriptions',
      jsonBody('POST', { saved_search_id: savedSearchId, channel }),
    );
  }

  /** Pause or resume a subscription. */
  async function setSubscriptionActive(id: number, active: boolean): Promise<Subscription> {
    return requestData<Subscription>(`/api/v1/me/subscriptions/${id}`, jsonBody('PATCH', { active }));
  }

  /** Unsubscribe by subscription id. */
  async function deleteSubscription(id: number): Promise<void> {
    await call(`/api/v1/me/subscriptions/${id}`, { method: 'DELETE' });
  }

  /** Whether Telegram notifications are configured and whether this user is linked. */
  async function telegramStatus(): Promise<TelegramStatus> {
    return requestData<TelegramStatus>('/api/v1/me/telegram');
  }

  /** Mint a one-time deep link the user opens to connect their Telegram chat. */
  async function telegramLink(): Promise<string> {
    const res = await request<{ data: { url: string } }>('/api/v1/me/telegram/link', { method: 'POST' });
    return res.data.url;
  }

  /** Disconnect the user's Telegram chat. */
  async function telegramUnlink(): Promise<void> {
    await call('/api/v1/me/telegram', { method: 'DELETE' });
  }

  /**
   * Whether this user has linked a Discord account for the paid channels.
   *
   * Throws a 404 when the deployment has no Discord application configured — the routes are
   * not mounted at all then. Callers hide the card on that, rather than showing an error for
   * a feature that simply is not offered here.
   */
  async function discordStatus(): Promise<DiscordStatus> {
    return requestData<DiscordStatus>('/api/v1/me/discord');
  }

  /** Disconnect the user's Discord account, giving up the paid role with it. */
  async function discordUnlink(): Promise<void> {
    await call('/api/v1/me/discord', { method: 'DELETE' });
  }

  /** Submit a vacancy for moderation. Returns the pending submission. */
  async function submitJob(input: SubmissionInput): Promise<Submission> {
    return requestData<Submission>(
      '/api/v1/submissions',
      jsonBody('POST', input satisfies components['schemas']['SubmissionRequest']),
    );
  }

  /** The caller's own submissions with their review status. */
  async function listMySubmissions(): Promise<Submission[]> {
    return requestData<Submission[]>('/api/v1/me/submissions');
  }

  /** Parse a job URL into draft field values for the submit form to prefill — never
   *  persisted. An unrecognized or non-vacancy URL comes back with every field empty,
   *  not an error. */
  async function prefillSubmission(url: string): Promise<PrefillResult> {
    return requestData<PrefillResult>('/api/v1/submissions/prefill', jsonBody('POST', { url }));
  }

  /** Ask whether a job page in the wild is one the catalog already carries. Public and
   *  read-only — it writes nothing, records no contribution, and fetches no page, so it is
   *  the half of the link intake a signed-out visitor may run. `null` means the URL names
   *  no posting we hold, which is an answer rather than an error. */
  async function findJobByUrl(url: string): Promise<FoundJob | null> {
    return requestData<FoundJob | null>(`/api/v1/jobs/find?url=${encodeURIComponent(url)}`);
  }

  /** Hand a job link to freehire. One sequence serves every surface: the catalog is checked,
   *  the vacancy imported when anything can read it, and the board behind it recorded for
   *  onboarding either way. The outcome says which of those happened (422 for a non-URL). */
  async function resolveJobLink(url: string): Promise<ResolvedLink> {
    return requestData<ResolvedLink>('/api/v1/jobs/resolve', jsonBody('POST', { url, surface: 'web' }));
  }

  /** The caller's own link contributions, newest first. */
  async function listMyContributions(): Promise<Contribution[]> {
    return requestData<Contribution[]>('/api/v1/me/contributions');
  }

  return {
    listSubscriptions,
    createSubscription,
    setSubscriptionActive,
    deleteSubscription,
    telegramStatus,
    telegramLink,
    telegramUnlink,
    discordStatus,
    discordUnlink,
    submitJob,
    listMySubmissions,
    prefillSubmission,
    findJobByUrl,
    resolveJobLink,
    listMyContributions,
  };
}
