import type { Http } from '../api-core';
import type {
  AiUsage,
  ApiKey,
  ApiPlan,
  ApiUsageDay,
  BillingOverview,
  CheckoutSession,
  CreatedApiKey,
  InviteSummary,
  NotificationItem,
  NotificationSettings,
  OAuthGrant,
  PlansMatrix,
  PlanState,
  SavedSearch,
  UsageHistoryEntry,
  User,
  WebhookConfig,
} from '../types';
import { track } from '../analytics';
import { jsonBody, query } from '../api-core';
import type { components } from '#lib/generated/api.js';

export function accountApi({ call, request, requestData }: Http) {
  /** The caller's plan and every metered feature's standing today. Never triggers the LLM.
   *  Powers the plan page. */
  async function myPlan(): Promise<PlanState> {
    return requestData<PlanState>('/api/v1/me/plan');
  }

  /** Where this caller buys Pro: a link to the provider's hosted paywall, carrying their
   *  own account id. We never see a card.
   *
   *  Throws when billing is not configured on this deployment, or when no paywall is set
   *  up — both answer 404. Callers treat that as "no upgrade offer here" and hide the
   *  entry point, never as an error to show. */
  async function billingCheckout(priceID?: string): Promise<CheckoutSession> {
    // Intent, recorded before the answer: this is the step between reaching the plan page
    // and paying, and it is the only one that says a visitor meant to buy. A failure to
    // create the session is then visible as intent without a purchase rather than as
    // nothing at all.
    track('checkout_start', { price_id: priceID ?? 'default' });
    const q = priceID ? `?price=${encodeURIComponent(priceID)}` : '';
    // No code here. This is a GET, and `SameSite=Lax` sends the session cookie on a
    // cross-site top-level navigation — so a GET that redeemed a code would let any page
    // burn a visitor's one lifetime redemption by linking to it. Redemption is its own
    // POST; this call only reads back what the account already holds.
    return requestData<CheckoutSession>(`/api/v1/billing/checkout${q}`);
  }

  /** Check what a promo code is worth without spending it. Every refusal about the code
   *  itself is the same 404 — telling "no such code" apart
   *  from "out of seats" would make this an oracle for guessing them. */
  async function promoPreview(code: string): Promise<{ percent_off: number }> {
    // jsonBody and not a hand-built init: it carries the Content-Type the server's body
    // parser requires. Without that header the request is refused before the code is even
    // read, which looks exactly like a code that does not exist.
    return requestData<{ percent_off: number }>('/api/v1/me/promo/preview', jsonBody('POST', { code }));
  }

  /** Spend this account's one lifetime redemption on a code. Durable: once recorded, every
   *  later checkout reads the percentage back, so a provider failure while opening the
   *  payment page costs a retry rather than the offer. */
  async function promoRedeem(code: string): Promise<{ percent_off: number }> {
    return requestData<{ percent_off: number }>('/api/v1/me/promo/redeem', jsonBody('POST', { code }));
  }

  /** This account's invite link and what it has earned. */
  async function myInvite(): Promise<InviteSummary> {
    return requestData<InviteSummary>('/api/v1/me/invite');
  }

  /** What the caller is paying and what has been charged. 404 when there is no
   *  subscription, which is the ordinary state for a free account — the surface renders
   *  nothing rather than an error. */
  async function billingSubscription(): Promise<BillingOverview> {
    return requestData<BillingOverview>('/api/v1/billing/subscription');
  }

  /** What each plan allows and what Pro costs. Public: no session needed. Powers /pricing.
   *
   *  The allowances come from the same configuration the metering path reads, so the
   *  comparison cannot drift from the limits actually enforced. */
  async function plans(): Promise<PlansMatrix> {
    return requestData<PlansMatrix>('/api/v1/plans');
  }

  /** Where this caller manages their subscription — the payment provider's own page, where
   *  they change their card or cancel. Generated per visit and short-lived, so it is fetched
   *  when a surface is about to show it rather than stored.
   *
   *  404 when there is no subscription; 500 when the provider is unreachable. Either way
   *  the surface omits the link rather than rendering a broken one. */
  async function billingManageUrl(): Promise<{ url: string }> {
    return requestData<{ url: string }>('/api/v1/billing/manage');
  }

  /** What the caller's account did this period: model calls, failures and tokens, read
   *  from the LLM gateway. Never fails for anything the caller can act on — an account
   *  that has never used AI, and a gateway that is down, both answer zeroes. */
  async function myUsage(): Promise<AiUsage> {
    return requestData<AiUsage>('/api/v1/me/usage');
  }

  /** The caller's usage history, newest first — what each metered action was spent on and
   *  what was given back, each labelled for display. Powers the plan page's list. */
  async function myPlanHistory(): Promise<UsageHistoryEntry[]> {
    return requestData<UsageHistoryEntry[]>('/api/v1/me/plan/history');
  }

  /** The public slugs of every job the current user has interacted with. The
   *  browse UI cross-references this set to dim already-viewed cards without
   *  authenticating the public job list. */
  async function listViewedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/viewed');
  }

  /** The public slugs of every job the current user has saved (bookmarked). The
   *  browse UI cross-references this set to render the save toggle as filled on
   *  already-saved cards without authenticating the public job list. */
  async function listSavedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/saved');
  }

  /** The company slugs of every employer the current user has applied to. The browse UI
   *  and the job page compare each posting's `company_slug` against it to mark "you
   *  already applied to this company" without authenticating the public job reads. */
  async function listAppliedCompanySlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/applied-companies');
  }

  // --- Notification settings -------------------------------------------------
  //
  // One account-level rule (enable, channels) gates saved-job reminders and both
  // lifecycle nudges (follow-up, interview-prep). Cookie-only.

  /** The caller's notification rule (on by default for a never-configured account). */
  async function getNotificationSettings(): Promise<NotificationSettings> {
    return requestData<NotificationSettings>('/api/v1/me/notification-settings');
  }

  /** Replace the caller's notification rule. An enabled rule needs at least one
   *  channel, else the server rejects it (400). */
  async function updateNotificationSettings(settings: NotificationSettings): Promise<NotificationSettings> {
    return requestData<NotificationSettings>(
      '/api/v1/me/notification-settings',
      jsonBody('PUT', settings satisfies components['schemas']['NotificationSettingsUpdate']),
    );
  }

  /** Sets the account's IANA timezone (e.g. "Europe/Moscow"). 400s on a name Go's
   *  tzdata does not recognize. Automatic adoption uses onlyIfUnset to preserve a
   *  concurrently saved preference. Returns the current user. */
  async function updateTimezone(timezone: string, onlyIfUnset = false): Promise<User> {
    return requestData<User>('/api/v1/me/timezone', jsonBody('PATCH', { timezone, only_if_unset: onlyIfUnset }));
  }

  /** Sets the account's preferred interface language (e.g. "ru"). 400s on a code
   *  outside the curated set. Returns the updated user (carries the new value). */
  async function updateLanguage(language: string): Promise<User> {
    return requestData<User>('/api/v1/me/language', jsonBody('PATCH', { language }));
  }

  /** The public slugs of every job the current user has hidden (dismissed). The
   *  browse UI cross-references this set to exclude hidden jobs from the feed
   *  without authenticating the public job list. */
  async function listDismissedSlugs(): Promise<string[]> {
    return requestData<string[]>('/api/v1/me/tracking/dismissed');
  }

  // --- Notification center ---------------------------------------------------
  //
  // The durable, readable record of every subscription-digest/reminder/nudge
  // delivered to the caller over any channel — independent of the notification
  // *rule* above, which only gates whether those events fire at all.

  /** A page of the caller's notification-center entries, newest first.
   *  `meta.unread_count` rides alongside the page (not a separate endpoint — see
   *  design.md's non-goals) so the bell badge can be refreshed from the same call
   *  that fetches a page. */
  async function getNotifications(
    limit = 20,
    offset = 0,
  ): Promise<{
    data: NotificationItem[];
    meta: { total: number; unread_count: number; limit: number; offset: number };
  }> {
    return request<{
      data: NotificationItem[];
      meta: { total: number; unread_count: number; limit: number; offset: number };
    }>(`/api/v1/me/notifications${query(limit, offset)}`);
  }

  /** One notification, including its `jobs` snapshot when it has one — the
   *  read a direct visit/bookmark of the digest jobs-list page needs, since
   *  getNotifications alone only ever serves the caller's current page. */
  async function getNotification(id: number): Promise<NotificationItem> {
    return requestData<NotificationItem>(`/api/v1/me/notifications/${id}`);
  }

  /** Mark one notification read. Idempotent; owner-scoped (404s for another
   *  user's, but that never happens from this client since ids come from the
   *  caller's own list). */
  async function markNotificationRead(id: number): Promise<void> {
    await call(`/api/v1/me/notifications/${id}/read`, { method: 'POST' });
  }

  /** Mark every currently-unread notification of the caller's read; returns the
   *  count marked. */
  async function markAllNotificationsRead(): Promise<{ marked: number }> {
    return requestData<{ marked: number }>('/api/v1/me/notifications/read-all', { method: 'POST' });
  }

  // --- API keys -------------------------------------------------------------
  //
  // Personal API keys for non-browser access. Management is cookie-only (these
  // calls ride the session cookie); the plaintext token is returned once, by
  // createApiKey, and never again.

  /** The caller's API product tier and the rate its full-scope keys get (cookie-only). */
  async function getApiPlan(): Promise<ApiPlan> {
    return requestData<ApiPlan>('/api/v1/me/api-plan');
  }

  /** The caller's API requests per UTC day over the last 30 days, oldest first; a day
   *  without requests is absent (cookie-only). */
  async function getApiUsage(): Promise<ApiUsageDay[]> {
    return requestData<ApiUsageDay[]>('/api/v1/me/api-usage');
  }

  /** The current user's API keys (metadata only — no secret). */
  async function listApiKeys(): Promise<ApiKey[]> {
    return requestData<ApiKey[]>('/api/v1/me/api-keys');
  }

  /** Create a key and return it with its one-time plaintext `token`. `expiresAt` is
   *  an RFC3339 string, or omitted for a key that never expires. `scope` is omitted
   *  for the default full-access key; the only other value a client may request is
   *  `"embed"` (for the volunteer-embed-mvp service — see that scope's own doc on
   *  the server). */
  async function createApiKey(name: string, expiresAt?: string, scope?: string): Promise<CreatedApiKey> {
    return requestData<CreatedApiKey>(
      '/api/v1/me/api-keys',
      jsonBody('POST', { name, expires_at: expiresAt ?? null, scope }),
    );
  }

  /** Revoke a key by id; it stops authenticating immediately. */
  async function revokeApiKey(id: number): Promise<void> {
    await call(`/api/v1/me/api-keys/${id}`, { method: 'DELETE' });
  }

  // --- MCP OAuth grants ("Connected devices") --------------------------------
  //
  // Clients (Claude, Cursor, ChatGPT) the user approved via the OAuth consent
  // screen, rather than a pasted API key. Management is cookie-only, same
  // reasoning as API keys.

  /** The current user's connected MCP clients (metadata only — no token). */
  async function listOAuthGrants(): Promise<OAuthGrant[]> {
    return requestData<OAuthGrant[]>('/api/v1/me/oauth-grants');
  }

  /** Revoke a connected client by grant id; it stops authenticating immediately. */
  async function revokeOAuthGrant(id: number): Promise<void> {
    await call(`/api/v1/me/oauth-grants/${id}`, { method: 'DELETE' });
  }

  // --- Webhook (saved-search alerts) -----------------------------------------
  //
  // The account's single webhook destination for saved-search matches (the
  // `webhook` subscription channel). Management is cookie-only. Deliveries
  // are plain, unsigned POSTs.

  /** The current user's webhook destination, or null if none is configured. */
  async function getWebhook(): Promise<WebhookConfig | null> {
    return requestData<WebhookConfig | null>('/api/v1/me/webhook');
  }

  /** Create the destination, or update its URL if one already exists. */
  async function createOrUpdateWebhook(url: string): Promise<WebhookConfig> {
    return requestData<WebhookConfig>('/api/v1/me/webhook', jsonBody('POST', { url }));
  }

  /** Enable or disable the destination without changing its URL. */
  async function setWebhookEnabled(enabled: boolean): Promise<WebhookConfig> {
    return requestData<WebhookConfig>('/api/v1/me/webhook', jsonBody('PATCH', { enabled }));
  }

  /** Send a sample delivery to the stored destination. A pass returns the
   *  destination with `last_success_at` stamped; a destination that refuses it
   *  is a 422 whose message names what it answered. Saving a URL runs the same
   *  test first. */
  async function testWebhook(): Promise<WebhookConfig> {
    return requestData<WebhookConfig>('/api/v1/me/webhook/test', { method: 'POST' });
  }

  /** Whether the account has a registered mobile device. Push delivers to
   *  nothing else, so the push channel is offered only once the mobile app has
   *  registered one (the server refuses it otherwise with 409). */
  async function hasPushDevice(): Promise<boolean> {
    const devices = await requestData<unknown[]>('/api/v1/me/push-tokens');
    return devices.length > 0;
  }

  /** Delete the destination entirely. */
  async function deleteWebhook(): Promise<void> {
    await call('/api/v1/me/webhook', { method: 'DELETE' });
  }

  // Saved searches: named snapshots of the filter state (cookie-only on the server).

  /** The current user's saved searches, most recently updated first. */
  async function listSavedSearches(): Promise<SavedSearch[]> {
    return requestData<SavedSearch[]>('/api/v1/me/searches');
  }

  /** Save the current filter state under a name. `queryString` is the canonical search
   *  query string (may be empty). A duplicate name or the per-user cap is a 409.
   *  `derivedFromProfile` marks the auto-generated "notify me about jobs matching my
   *  profile" search; a second one for the same user is a 409. */
  async function createSavedSearch(
    name: string,
    queryString: string,
    derivedFromProfile = false,
  ): Promise<SavedSearch> {
    return requestData<SavedSearch>(
      '/api/v1/me/searches',
      jsonBody('POST', { name, query: queryString, derived_from_profile: derivedFromProfile }),
    );
  }

  /** Overwrite a saved search's name and/or query; an omitted field is unchanged. */
  async function updateSavedSearch(id: number, patch: { name?: string; query?: string }): Promise<SavedSearch> {
    return requestData<SavedSearch>(`/api/v1/me/searches/${id}`, jsonBody('PATCH', patch));
  }

  /** Delete a saved search by id. */
  async function deleteSavedSearch(id: number): Promise<void> {
    await call(`/api/v1/me/searches/${id}`, { method: 'DELETE' });
  }

  return {
    myPlan,
    billingCheckout,
    promoPreview,
    promoRedeem,
    myInvite,
    billingSubscription,
    plans,
    billingManageUrl,
    myUsage,
    myPlanHistory,
    listViewedSlugs,
    listSavedSlugs,
    listAppliedCompanySlugs,
    getNotificationSettings,
    updateNotificationSettings,
    updateTimezone,
    updateLanguage,
    listDismissedSlugs,
    getNotifications,
    getNotification,
    markNotificationRead,
    markAllNotificationsRead,
    getApiPlan,
    getApiUsage,
    listApiKeys,
    createApiKey,
    revokeApiKey,
    listOAuthGrants,
    revokeOAuthGrant,
    getWebhook,
    createOrUpdateWebhook,
    setWebhookEnabled,
    testWebhook,
    hasPushDevice,
    deleteWebhook,
    listSavedSearches,
    createSavedSearch,
    updateSavedSearch,
    deleteSavedSearch,
  };
}
