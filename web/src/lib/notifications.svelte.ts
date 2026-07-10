// The signed-in user's notification state: their Telegram link status and their
// filter subscriptions (one per saved search + channel). Read once for an
// authenticated user (the filters panel triggers the load); subscribe/unsubscribe
// and link/unlink keep the local state in sync so the toggle updates without a
// reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and leaves empty/disabled state for signed-out users.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { Subscription, TelegramStatus } from '$lib/types';

const disabled: TelegramStatus = { enabled: false, linked: false };

class Notifications extends UserResource<[TelegramStatus, Subscription[]]> {
  #telegram = $state.raw<TelegramStatus>(disabled);
  #subs = $state.raw<Subscription[]>([]);

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

  protected load(): Promise<[TelegramStatus, Subscription[]]> {
    return Promise.all([api.telegramStatus(), api.listSubscriptions()]);
  }

  protected apply([tg, subs]: [TelegramStatus, Subscription[]]) {
    this.#telegram = tg;
    this.#subs = subs;
  }

  protected clearState() {
    this.#telegram = disabled;
    this.#subs = [];
  }

  /** Subscribe a saved search to Telegram; prepend the new subscription. */
  async subscribe(savedSearchId: number): Promise<void> {
    const sub = await api.createSubscription(savedSearchId);
    this.#subs = [sub, ...this.#subs];
  }

  /** Pause/resume a subscription in place. */
  async setActive(id: number, active: boolean): Promise<void> {
    const row = await api.setSubscriptionActive(id, active);
    this.#subs = this.#subs.map((s) => (s.id === id ? { ...s, active: row.active } : s));
  }

  /** Unsubscribe and drop it from the list. */
  async unsubscribe(id: number): Promise<void> {
    await api.deleteSubscription(id);
    this.#subs = this.#subs.filter((s) => s.id !== id);
  }

  /** Mint the deep link the user opens to connect their Telegram chat. */
  link(): Promise<string> {
    return api.telegramLink();
  }

  /** Re-read the link status (after the user reports they connected the bot). */
  async refreshTelegram(): Promise<void> {
    this.#telegram = await api.telegramStatus();
  }

  /** Disconnect Telegram. */
  async unlink(): Promise<void> {
    await api.telegramUnlink();
    this.#telegram = { ...this.#telegram, linked: false, chat_id: undefined };
  }
}

export const notifications = new Notifications();
