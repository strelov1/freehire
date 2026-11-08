import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const { trackPlanRefusal } = vi.hoisted(() => ({ trackPlanRefusal: vi.fn() }));
vi.mock('#lib/api.js', () => ({ trackPlanRefusal }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));

import ReferralMessageDialog from './ReferralMessageDialog.svelte';

beforeEach(() => {
  trackPlanRefusal.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

it('sends pasted text for one draft and lets the seeker edit before copying', async () => {
  const request = vi.fn().mockResolvedValue(new Response(
    'event: message\ndata: {"body":"Hello Sam. Would you consider a referral?","cited":[]}\n\n',
    { status: 200, headers: { 'content-type': 'text/event-stream' } },
  ));
  vi.stubGlobal('fetch', request);
  const copied = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copied } });
  render(ReferralMessageDialog, { slug: 'acme-role', company: 'Acme', onClose: vi.fn() });

  await fireEvent.input(screen.getByLabelText("Contact's public profile text"), { target: { value: 'Sam, engineer at Acme' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Draft message' }));
  await waitFor(() => expect(screen.getByLabelText('Your message — edit before sending')).not.toBeNull());
  expect(request).toHaveBeenCalledOnce();
  expect(JSON.parse(request.mock.lastCall?.[1]?.body ?? '{}')).toEqual({ channel: 'linkedin', profile: 'Sam, engineer at Acme' });

  await fireEvent.input(screen.getByLabelText('Your message — edit before sending'), { target: { value: 'Hello Sam, may I ask for a referral?' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Copy message' }));
  expect(copied).toHaveBeenCalledWith('Hello Sam, may I ask for a referral?');
});

it('shows the paid-plan link when drafting is refused', async () => {
  const refusal = { error: 'A paid plan is required.', upgrade_url: '/pricing' };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(refusal), {
    status: 402,
    headers: { 'content-type': 'application/json' },
  })));
  render(ReferralMessageDialog, { slug: 'acme-role', company: 'Acme', onClose: vi.fn() });

  await fireEvent.input(screen.getByLabelText("Contact's public profile text"), { target: { value: 'Sam at Acme' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Draft message' }));

  await waitFor(() => expect(screen.getByRole('alert').textContent).toContain(refusal.error));
  expect(screen.getByRole('link', { name: 'See paid plans' }).getAttribute('href')).toBe('/pricing');
  expect(trackPlanRefusal).toHaveBeenCalledWith(402, refusal);
});
