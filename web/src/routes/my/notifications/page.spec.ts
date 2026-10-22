import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { NotificationItem } from '#lib/types.js';
import NotificationsPage from './+page.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'ru' }, url: new URL('http://localhost/') },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

const { getNotifications, markAllRead } = vi.hoisted(() => ({
  getNotifications: vi.fn(),
  markAllRead: vi.fn(),
}));

vi.mock('#lib/api.js', () => ({ api: { getNotifications } }));
vi.mock('#lib/notificationCenter.svelte.js', () => ({
  notificationCenter: { markAllRead },
}));

const unread: NotificationItem = {
  id: 1,
  kind: 'reminder',
  title: 'A reminder',
  body: 'Body',
  public_slug: null,
  created_at: new Date().toISOString(),
  read_at: null,
};

beforeEach(() => {
  getNotifications.mockReset();
  markAllRead.mockReset();
});

describe('/my/notifications — Russian locale', () => {
  it('renders "mark all read" in Russian when unread notifications exist', async () => {
    getNotifications.mockResolvedValue({
      data: [unread],
      meta: { total: 1, unread_count: 1, limit: 20, offset: 0 },
    });
    render(NotificationsPage);

    await vi.waitFor(() => expect(screen.getByText('Отметить всё прочитанным')).toBeTruthy());
  });

  it('renders the empty state in Russian when there are no notifications', async () => {
    getNotifications.mockResolvedValue({
      data: [],
      meta: { total: 0, unread_count: 0, limit: 20, offset: 0 },
    });
    render(NotificationsPage);

    await vi.waitFor(() => expect(screen.getByText('Пока нет уведомлений.')).toBeTruthy());
  });
});
