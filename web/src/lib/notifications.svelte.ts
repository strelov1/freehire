// The signed-in user's notification state: their Telegram link status and their
// filter subscriptions (one per saved search + channel). Read once for an
// authenticated user (the filters panel triggers the load); subscribe/unsubscribe
// and link/unlink keep the local state in sync so the toggle updates without a
// reload.
//
// SSR-safe and auth-agnostic, mirroring savedSearches: `ensureLoaded` is a no-op
// off the browser and leaves empty/disabled state for signed-out users.

import { browser } from '$app/environment';
import {
  listSubscriptions,
  createSubscription,
  setSubscriptionActive,
  deleteSubscription,
  telegramStatus,
  telegramLink,
  telegramUnlink,
} from '$lib/api';
import type { Subscription, TelegramStatus } from '$lib/types';

const disabled: TelegramStatus = { enabled: false, linked: false };

class Notifications {
  #telegram = $state.raw<TelegramStatus>(disabled);
  #subs = $state.raw<Subscription[]>([]);
  #loaded = false;
  #loading: Promise<void> | null = null;

  get telegram(): TelegramStatus {
    return this.#telegram;
  }

  get subscriptions(): Subscription[] {
    return this.#subs;
  }

  /** The telegram subscription for a saved search, if any. */
  forSavedSearch(savedSearchId: number): Subscription | undefined {
    return this.#subs.find((s) => s.saved_search_id === savedSearchId && s.channel === 'telegram');
  }

  /** Load status + subscriptions once. No-op on the server; a failed load leaves
   *  the feature looking disabled (best-effort, never breaks the page). */
  async ensureLoaded(): Promise<void> {
    if (!browser || this.#loaded) return;
    if (this.#loading) return this.#loading;
    this.#loading = Promise.all([telegramStatus(), listSubscriptions()])
      .then(([tg, subs]) => {
        this.#telegram = tg;
        this.#subs = subs;
        this.#loaded = true;
      })
      .catch(() => {})
      .finally(() => {
        this.#loading = null;
      });
    return this.#loading;
  }

  /** Subscribe a saved search to Telegram; prepend the new subscription. */
  async subscribe(savedSearchId: number): Promise<void> {
    const sub = await createSubscription(savedSearchId);
    this.#subs = [sub, ...this.#subs];
  }

  /** Pause/resume a subscription in place. */
  async setActive(id: number, active: boolean): Promise<void> {
    const row = await setSubscriptionActive(id, active);
    this.#subs = this.#subs.map((s) => (s.id === id ? { ...s, active: row.active } : s));
  }

  /** Unsubscribe and drop it from the list. */
  async unsubscribe(id: number): Promise<void> {
    await deleteSubscription(id);
    this.#subs = this.#subs.filter((s) => s.id !== id);
  }

  /** Mint the deep link the user opens to connect their Telegram chat. */
  link(): Promise<string> {
    return telegramLink();
  }

  /** Re-read the link status (after the user reports they connected the bot). */
  async refreshTelegram(): Promise<void> {
    this.#telegram = await telegramStatus();
  }

  /** Disconnect Telegram. */
  async unlink(): Promise<void> {
    await telegramUnlink();
    this.#telegram = { ...this.#telegram, linked: false, chat_id: undefined };
  }

  /** Drop cached state on sign-out so the next user loads their own. */
  reset() {
    this.#telegram = disabled;
    this.#subs = [];
    this.#loaded = false;
    this.#loading = null;
  }
}

export const notifications = new Notifications();
