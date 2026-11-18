import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ReminderSettings from './ReminderSettings.svelte';

// Stub the facade instead of initializing unrelated UI exports.
vi.mock('#lib/ui/index.js', () => ({
  Badge() {},
  cn: (...classes: unknown[]) => classes.filter(Boolean).join(' '),
  ProviderIcon() {},
}));
const navigation = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', () => navigation);
vi.mock('@lucide/svelte', async () => ({
  Bell: (await import('@lucide/svelte/icons/bell')).default,
  Check: (await import('@lucide/svelte/icons/check')).default,
  Mail: (await import('@lucide/svelte/icons/mail')).default,
  Moon: (await import('@lucide/svelte/icons/moon')).default,
  Smartphone: (await import('@lucide/svelte/icons/smartphone')).default,
}));

const backend = vi.hoisted(() => ({
  getNotificationSettings: vi.fn(),
  updateNotificationSettings: vi.fn(),
  telegramStatus: vi.fn(),
  hasPushDevice: vi.fn(),
  myPlan: vi.fn(),
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
  faster_alerts_available: true,
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
  backend.myPlan.mockResolvedValue({ plan: 'pro' });
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

  it('lets a free candidate remove consent while global notifications are off', async () => {
    backend.myPlan.mockResolvedValue({ plan: 'free' });
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

it('sends a free account to the plans instead of enabling the inbox summary', async () => {
  backend.myPlan.mockResolvedValue({ plan: 'free' });
  render(ReminderSettings);
  const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
  expect(toggle.hasAttribute('disabled')).toBe(false);
  expect(screen.getByRole('link', { name: 'Pro feature, view plans' }).getAttribute('href')).toBe('/pricing');
  await fireEvent.click(toggle);
  expect(navigation.goto).toHaveBeenCalledWith('/pricing');
  expect(toggle.getAttribute('aria-checked')).toBe('false');
  await new Promise((resolve) => setTimeout(resolve, 450)); // past the autosave debounce
  expect(backend.updateNotificationSettings).not.toHaveBeenCalled();
});

it('allows Ultra to opt in', async () => {
  backend.myPlan.mockResolvedValue({ plan: 'ultra' });
  render(ReminderSettings);
  const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
  expect(toggle.hasAttribute('disabled')).toBe(false);
});

it('sends a later change only after the earlier save finishes', async () => {
  let finishFirst = () => {};
  backend.updateNotificationSettings.mockImplementationOnce(
    () => new Promise((resolve) => (finishFirst = () => resolve({ ...settings }))),
  );
  render(ReminderSettings);
  const toggle = await screen.findByRole('switch', { name: 'Daily inbox summary' });
  await fireEvent.click(toggle);
  await vi.waitFor(() => expect(backend.updateNotificationSettings).toHaveBeenCalledTimes(1));
  await fireEvent.click(toggle);
  await new Promise((resolve) => setTimeout(resolve, 450)); // past the autosave debounce
  expect(backend.updateNotificationSettings).toHaveBeenCalledTimes(1);
  finishFirst();
  await vi.waitFor(() =>
    expect(backend.updateNotificationSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({ inbox_digest_enabled: false }),
    ),
  );
  expect(backend.updateNotificationSettings).toHaveBeenCalledTimes(2);
});

describe('Weekly application summary settings', () => {
  it('requires independent consent and keeps the chosen time for batched search alerts', async () => {
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Weekly application summary' });
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(toggle);
    const time = screen.getByLabelText('Daily digest time');
    await fireEvent.input(time, { target: { value: '14:30' } });
    await fireEvent.change(time);
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenLastCalledWith(
        expect.objectContaining({
          application_digest_enabled: true,
          inbox_digest_enabled: false,
          digest_time: '14:30',
          digest_frequency: 'interval',
        }),
      ),
    );
    expect(screen.getByText(/Every Monday at the digest time/)).toBeTruthy();
  });

  it('allows withdrawing consent after a subscription ends and notifications are disabled', async () => {
    backend.myPlan.mockResolvedValue({ plan: 'free' });
    backend.getNotificationSettings.mockResolvedValue({
      ...settings,
      enabled: false,
      application_digest_enabled: true,
    });
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Weekly application summary' });
    expect(toggle.hasAttribute('disabled')).toBe(false);
    await fireEvent.click(toggle);
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ application_digest_enabled: false, enabled: false }),
      ),
    );
  });

  it('sends a free account to plans without saving consent', async () => {
    backend.myPlan.mockResolvedValue({ plan: 'free' });
    render(ReminderSettings);
    const toggle = await screen.findByRole('switch', { name: 'Weekly application summary' });
    await fireEvent.click(toggle);
    expect(navigation.goto).toHaveBeenCalledWith('/pricing');
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    expect(backend.updateNotificationSettings).not.toHaveBeenCalled();
  });

  it('requires email delivery before new consent', async () => {
    backend.getNotificationSettings.mockResolvedValue({ ...settings, channels: ['telegram'] });
    render(ReminderSettings);
    expect((await screen.findByRole('switch', { name: 'Weekly application summary' })).hasAttribute('disabled')).toBe(
      true,
    );
  });
});

describe('Search alert frequency', () => {
  const freeWithHistoricalInstant = { ...settings, digest_frequency: 'instant', faster_alerts_available: false };

  it('shows a free account its real daily schedule and keeps the stored preference on unrelated edits', async () => {
    backend.myPlan.mockResolvedValue({ plan: 'free' });
    backend.getNotificationSettings.mockResolvedValue({ ...freeWithHistoricalInstant });
    render(ReminderSettings);
    const daily = await screen.findByRole('button', { name: 'Daily digest' });
    expect(daily.getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', { name: /Instant/ }).getAttribute('aria-pressed')).toBe('false');
    expect(screen.getByLabelText('Daily digest time')).toBeTruthy();
    expect(screen.getByText(/Free accounts get new matches in one daily digest/)).toBeTruthy();

    await fireEvent.click(screen.getByRole('switch', { name: 'Enable quiet hours' }));
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ digest_frequency: 'instant', digest_time: '09:00' }),
      ),
    );
  });

  it('sends a free account to the plans for a faster frequency without saving anything', async () => {
    backend.myPlan.mockResolvedValue({ plan: 'free' });
    backend.getNotificationSettings.mockResolvedValue({
      ...settings,
      digest_frequency: 'daily',
      faster_alerts_available: false,
    });
    render(ReminderSettings);
    for (const name of [/Instant/, /Batched/]) {
      await fireEvent.click(await screen.findByRole('button', { name }));
    }
    expect(navigation.goto).toHaveBeenCalledTimes(2);
    expect(navigation.goto).toHaveBeenCalledWith('/pricing');
    expect(screen.getByRole('button', { name: 'Daily digest' }).getAttribute('aria-pressed')).toBe('true');
    await new Promise((resolve) => setTimeout(resolve, 450)); // past the autosave debounce
    expect(backend.updateNotificationSettings).not.toHaveBeenCalled();
  });

  it('lets a free account choose daily explicitly', async () => {
    backend.getNotificationSettings.mockResolvedValue({ ...freeWithHistoricalInstant });
    render(ReminderSettings);
    await fireEvent.click(await screen.findByRole('button', { name: 'Daily digest' }));
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ digest_frequency: 'daily' }),
      ),
    );
  });

  it('lets a paid account choose and keep a faster frequency', async () => {
    backend.getNotificationSettings.mockResolvedValue({ ...settings, digest_frequency: 'daily' });
    render(ReminderSettings);
    await fireEvent.click(await screen.findByRole('button', { name: /Instant/ }));
    await vi.waitFor(() =>
      expect(backend.updateNotificationSettings).toHaveBeenCalledWith(
        expect.objectContaining({ digest_frequency: 'instant' }),
      ),
    );
    expect(navigation.goto).not.toHaveBeenCalled();
    expect(screen.queryByText(/Free accounts get new matches/)).toBeNull();
  });
});
