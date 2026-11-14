import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ReminderSettings from './ReminderSettings.svelte';

const backend = vi.hoisted(() => ({
  getNotificationSettings: vi.fn(),
  updateNotificationSettings: vi.fn(),
  telegramStatus: vi.fn(),
  hasPushDevice: vi.fn(),
}));

vi.mock('#lib/api.js', () => ({ api: backend, ApiError: class extends Error {} }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => true,
  currentUser: () => ({ timezone: 'America/Sao_Paulo' }),
}));

const settings = {
  enabled: true,
  channels: ['email'],
  digest_frequency: 'interval',
  digest_time: '09:00',
  digest_interval_hours: 1,
  quiet_hours_start: null,
  quiet_hours_end: null,
  inbox_digest_enabled: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  backend.getNotificationSettings.mockResolvedValue({ ...settings });
  backend.updateNotificationSettings.mockResolvedValue({ ...settings });
  backend.telegramStatus.mockResolvedValue({ enabled: false });
  backend.hasPushDevice.mockResolvedValue(false);
});

describe('Daily inbox summary settings', () => {
  it('requires opting in and saves a daily time even when search alerts are batched', async () => {
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(toggle);
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ inbox_digest_enabled: true, digest_time: '09:00', digest_frequency: 'interval' }),
      ),
    );
  });

  it('lets a candidate remove consent while global notifications are off', async () => {
    backend.getNotificationSettings.mockResolvedValue({ ...settings, enabled: false, inbox_digest_enabled: true });
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
    expect(toggle.getAttribute('disabled')).toBeNull();
    await fireEvent.click(toggle);
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ enabled: false, inbox_digest_enabled: false }),
      ),
    );
  });

  it('requires an email delivery channel before new consent', async () => {
    backend.getNotificationSettings.mockResolvedValue({ ...settings, channels: ['telegram'] });
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
    expect(toggle.hasAttribute('disabled')).toBe(true);
  });
});
