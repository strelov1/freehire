import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, expect, it, vi } from 'vitest';
import type { MyJob } from '#lib/types.js';
import { createRawSnippet } from 'svelte';
import BoardPreview from './BoardPreview.svelte';

const children = createRawSnippet(() => ({ render: () => '<button type="button">Open Engineer at Acme</button>' }));

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' } } }));
const getTrackedApplication = vi.hoisted(() => vi.fn());
vi.mock('#lib/api.js', () => ({ api: { getTrackedApplication } }));
const item: MyJob = {
  id: 'role', company_slug: 'acme', role_title: 'Engineer',
  job: { public_slug: 'role', company: 'Acme', title: 'Engineer', closed_at: undefined },
  viewed_at: '', saved_at: '2026-10-01T00:00:00Z', applied_at: null, stage: null,
  notes: null, email_count: 0, last_activity_at: null, days_silent: null,
  silence_state: null, followed_up_at: null, cv_opened_at: null,
};
beforeEach(() => getTrackedApplication.mockReset());

it('fetches a plain-text preview on keyboard focus, caches it, and dismisses on Escape', async () => {
  getTrackedApplication.mockResolvedValue({ job: { description: '<p>Build &amp; maintain</p><p>reliable systems.</p>' } });
  render(BoardPreview, { item, children });
  expect(getTrackedApplication).not.toHaveBeenCalled();
  const card = screen.getByRole('button', { name: 'Open Engineer at Acme' });
  await fireEvent.focusIn(card);
  await waitFor(() => expect(screen.getByRole('tooltip').textContent).toContain('Build & maintain reliable systems.'));
  expect(getTrackedApplication).toHaveBeenCalledWith('role');
  expect(card.getAttribute('aria-describedby')).toContain(screen.getByRole('tooltip').id);
  await fireEvent.keyDown(window, { key: 'Escape' });
  expect(screen.queryByRole('tooltip')).toBeNull();
  await fireEvent.focusIn(card);
  await waitFor(() => expect(screen.getByRole('tooltip')).toBeTruthy());
  expect(getTrackedApplication).toHaveBeenCalledTimes(1);
});

it('shows unavailable on failure, and permits a later retry', async () => {
  getTrackedApplication.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ job: { description: 'Recovered' } });
  render(BoardPreview, { item, children });
  const card = screen.getByRole('button', { name: 'Open Engineer at Acme' });
  await fireEvent.focusIn(card);
  await waitFor(() => expect(screen.getByRole('tooltip').textContent).toContain('Description unavailable'));
  await fireEvent.keyDown(window, { key: 'Escape' });
  await fireEvent.focusIn(card);
  await waitFor(() => expect(screen.getByRole('tooltip').textContent).toContain('Recovered'));
});

it('limits the preview and never interprets description markup as live HTML', async () => {
  getTrackedApplication.mockResolvedValue({ job: { description: '<script>malicious()</script><p>' + 'x'.repeat(250) + '</p>' } });
  render(BoardPreview, { item, children });
  await fireEvent.focusIn(screen.getByRole('button', { name: 'Open Engineer at Acme' }));
  await waitFor(() => expect(screen.getByRole('tooltip').textContent?.trim()).toBe('x'.repeat(200) + '…'));
  expect(screen.getByRole('tooltip').querySelector('script')).toBeNull();
});

it('does not reopen on pointer-induced focus, but still opens for keyboard focus', async () => {
  getTrackedApplication.mockResolvedValue({ job: { description: 'Keyboard preview' } });
  render(BoardPreview, { item, children });
  const card = screen.getByRole('button', { name: 'Open Engineer at Acme' });
  await fireEvent.pointerDown(card);
  await fireEvent.focusIn(card);
  expect(screen.queryByRole('tooltip')).toBeNull();
  expect(getTrackedApplication).not.toHaveBeenCalled();
  await fireEvent.pointerUp(window);
  await fireEvent.focusOut(card);
  await fireEvent.focusIn(card);
  await waitFor(() => expect(screen.getByRole('tooltip').textContent).toContain('Keyboard preview'));
});
