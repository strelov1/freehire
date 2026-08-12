// The signed-in user's notification-center state: their unread count and the most
// recent page of `user_notifications` rows (subscription digests, reminders, and
// application nudges recorded at delivery time — see openspec/changes/
// add-notification-center). One module-level singleton, read by the header bell
// and its dropdown panel.
//
// Distinct from notifications.svelte.ts (that file owns the *rule* — Telegram link
// status and per-search channel subscriptions, i.e. whether these events fire at
// all — not the durable record of what already fired).
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op
// and leaves empty/zero state for signed-out users.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { NotificationItem } from '$lib/types';

const PAGE_LIMIT = 20;

interface Page {
  items: NotificationItem[];
  unreadCount: number;
}

class NotificationCenter extends UserResource<Page> {
  #items = $state.raw<NotificationItem[]>([]);
  #unreadCount = $state(0);

  get items(): NotificationItem[] {
    return this.#items;
  }

  get unreadCount(): number {
    return this.#unreadCount;
  }

  protected async load(): Promise<Page> {
    const res = await api.getNotifications(PAGE_LIMIT, 0);
    return { items: res.data, unreadCount: res.meta.unread_count };
  }

  protected apply({ items, unreadCount }: Page): void {
    this.#items = items;
    this.#unreadCount = unreadCount;
  }

  protected clearState(): void {
    this.#items = [];
    this.#unreadCount = 0;
  }

  /** Re-fetch the first page and unread count. Unlike ensureLoaded() (load-once),
   *  this always hits the network — called on the bell opening and after
   *  mark-all-read, so the badge and list reflect what the server has right now. */
  async refresh(): Promise<void> {
    const page = await this.load();
    this.apply(page);
    this.markLoaded();
  }

  /** Mark one notification read: always calls the server (idempotent — safe even
   *  if already read) and, if it was locally unread, clears its dot and decrements
   *  the badge without a full refetch. */
  async markRead(id: number): Promise<void> {
    await api.markNotificationRead(id);
    const wasUnread = this.#items.some((n) => n.id === id && n.read_at == null);
    if (!wasUnread) return;
    const readAt = new Date().toISOString();
    this.#items = this.#items.map((n) => (n.id === id ? { ...n, read_at: readAt } : n));
    this.#unreadCount = Math.max(0, this.#unreadCount - 1);
  }

  /** Mark every unread notification read in one call, then reconcile local state. */
  async markAllRead(): Promise<void> {
    await api.markAllNotificationsRead();
    const readAt = new Date().toISOString();
    this.#items = this.#items.map((n) => (n.read_at == null ? { ...n, read_at: readAt } : n));
    this.#unreadCount = 0;
  }
}

export const notificationCenter = new NotificationCenter();
