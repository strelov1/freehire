import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import NotificationBell from './NotificationBell.svelte';

const center = vi.hoisted(() => ({
  unreadCount: 0,
  items: [],
  loaded: true,
  ensureLoaded: vi.fn(),
  refresh: vi.fn(),
  reset: vi.fn(),
  markAllRead: vi.fn(),
}));

vi.mock('$app/navigation', () => ({ afterNavigate: () => {} }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true }));
vi.mock('#lib/notificationCenter.svelte.js', () => ({ notificationCenter: center }));

describe('NotificationBell — accessible name', () => {
  it('is plain "Notifications" with nothing unread', () => {
    center.unreadCount = 0;
    render(NotificationBell);

    expect(screen.getByRole('button', { name: 'Notifications' })).toBeTruthy();
  });

  it('carries the unread count the badge shows only visually', () => {
    center.unreadCount = 3;
    render(NotificationBell);

    const button = screen.getByRole('button', { name: 'Notifications, 3 unread' });
    expect(button.getAttribute('aria-haspopup')).toBe('dialog');
  });
});
