import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { t, tokenLabel } from '#lib/i18n/t.js';
import { dollarsToCents } from '#lib/sponsored.js';
import { messages } from './Sponsored.messages.js';
import PromoteJobForm from './PromoteJobForm.svelte';

const api = vi.hoisted(() => ({
  getJob: vi.fn(),
  createSponsoredCampaign: vi.fn(),
  topupSponsoredCampaign: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({
  ApiError: class ApiError extends Error {},
  api,
}));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/'), state: {} } }));

const open = { public_slug: 'acme-job', title: 'Engineer', company: 'Acme' };

describe('labels', () => {
  it('words every campaign status and invalid-click reason in plain language, in both locales', () => {
    for (const locale of ['en', 'ru'] as const) {
      const s = t(messages, locale);
      for (const k of [
        'draft',
        'pending_payment',
        'pending_review',
        'active',
        'paused',
        'exhausted',
        'rejected',
        'ended',
      ]) {
        expect(tokenLabel(s.status, k)).not.toBe(k);
      }
      for (const k of ['bot', 'duplicate', 'rate', 'inactive', 'budget']) {
        expect(tokenLabel(s.reason, k)).not.toBe(k);
      }
    }
  });

  it('shows an unknown token as itself', () => {
    expect(tokenLabel(t(messages, 'en').status, 'new_status')).toBe('new_status');
  });

  it('parses dollars to cents', () => {
    expect(dollarsToCents('0.50')).toBe(50);
    expect(dollarsToCents('$29')).toBe(2900);
    expect(dollarsToCents('1,25')).toBe(125);
    expect(dollarsToCents('')).toBeNull();
    expect(dollarsToCents('abc')).toBeNull();
    expect(dollarsToCents('-1')).toBeNull();
  });
});

describe('PromoteJobForm', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.getJob.mockResolvedValue(open);
  });

  it('shows the API error inline and does not start a payment', async () => {
    api.createSponsoredCampaign.mockRejectedValue(new Error('cpc below minimum'));
    render(PromoteJobForm, { slug: 'acme-job' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue to payment' }));
    // A plain Error is not an ApiError, so the generic text is shown.
    expect((await screen.findByRole('alert')).textContent).toContain('Could not start the campaign.');
    expect(api.topupSponsoredCampaign).not.toHaveBeenCalled();
  });

  it('refuses a non-numeric price without calling the API', async () => {
    render(PromoteJobForm, { slug: 'acme-job' });
    await fireEvent.input(await screen.findByLabelText(/Price per click/), { target: { value: 'free' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Continue to payment' }));
    expect((await screen.findByRole('alert')).textContent).toContain('positive dollar amounts');
    expect(api.createSponsoredCampaign).not.toHaveBeenCalled();
  });

  it('creates, tops up and reports a payment failure with the campaign kept', async () => {
    api.createSponsoredCampaign.mockResolvedValue({ id: 5 });
    api.topupSponsoredCampaign.mockRejectedValue(new Error('x'));
    render(PromoteJobForm, { slug: 'acme-job' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Continue to payment' }));
    await waitFor(() => expect(api.topupSponsoredCampaign).toHaveBeenCalledWith(5, 2900));
    expect((await screen.findByRole('alert')).textContent).toContain('campaign was created');
  });

  it('refuses a closed job', async () => {
    api.getJob.mockResolvedValue({ ...open, closed_at: '2026-01-01' });
    render(PromoteJobForm, { slug: 'acme-job' });
    expect((await screen.findByRole('alert')).textContent).toContain('closed');
    expect(screen.queryByRole('button', { name: 'Continue to payment' })).toBeNull();
  });
});
