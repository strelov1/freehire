import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const { findRecruiterContacts, user } = vi.hoisted(() => ({
  findRecruiterContacts: vi.fn(),
  user: { current: null as { tier: string } | null },
}));
vi.mock('#lib/api.js', () => ({ api: { findRecruiterContacts, plans: vi.fn().mockResolvedValue({ prices: [] }) } }));
vi.mock('#lib/auth.svelte.js', () => ({ currentUser: () => user.current }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
vi.mock('#lib/i18n/currentLocale.svelte', () => ({ locale: () => 'en' }));

import RecruiterContactsBlock from './RecruiterContactsBlock.svelte';

const props = { jobSlug: 'eng-acme', companyName: 'Acme' };

beforeEach(() => {
  findRecruiterContacts.mockReset();
  user.current = { tier: 'pro' };
});

it('offers a free reader the plans and never looks anything up', () => {
  user.current = { tier: 'free' };
  render(RecruiterContactsBlock, props);
  expect(screen.getByRole('link', { name: 'View plans' })).not.toBeNull();
  expect(screen.queryByRole('button', { name: 'Show recruiter contacts' })).toBeNull();
  expect(findRecruiterContacts).not.toHaveBeenCalled();
});

it('looks up on click and shows each contact with its confidence, a copy button and the note', async () => {
  findRecruiterContacts.mockResolvedValue([
    { name: 'Anna Lee', title: 'Technical Recruiter', email: 'anna@acme.io', confidence: 'high' },
    { name: 'Bob Ray', title: 'Talent Partner', email: 'bob@acme.io', confidence: 'medium' },
  ]);
  const copied = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copied } });
  render(RecruiterContactsBlock, props);

  await fireEvent.click(screen.getByRole('button', { name: 'Show recruiter contacts' }));
  expect(await screen.findByText('Anna Lee')).not.toBeNull();
  expect(findRecruiterContacts).toHaveBeenCalledWith('eng-acme');
  expect(screen.getByText('Verified')).not.toBeNull();
  expect(screen.getByText('Likely')).not.toBeNull();
  expect(screen.getByRole('link', { name: 'anna@acme.io' }).getAttribute('href')).toBe('mailto:anna@acme.io');
  expect(screen.getByText('Found from public sources. Write personally, do not mass-mail.')).not.toBeNull();

  await fireEvent.click(screen.getByRole('button', { name: 'Copy bob@acme.io' }));
  expect(copied).toHaveBeenCalledWith('bob@acme.io');
});

it('says so when nobody is known', async () => {
  findRecruiterContacts.mockResolvedValue([]);
  render(RecruiterContactsBlock, props);
  await fireEvent.click(screen.getByRole('button', { name: 'Show recruiter contacts' }));
  expect(await screen.findByText('Nobody who recruits at Acme is known yet. Nothing was charged.')).not.toBeNull();
});

it("shows the server's refusal with the plan link, and a failure as unavailable with a retry", async () => {
  findRecruiterContacts.mockRejectedValueOnce(new ApiError(402, "You've used today's recruiter contacts."));
  render(RecruiterContactsBlock, props);
  await fireEvent.click(screen.getByRole('button', { name: 'Show recruiter contacts' }));
  expect(await screen.findByText("You've used today's recruiter contacts.")).not.toBeNull();
  expect(screen.getByRole('link', { name: 'See your plan' })).not.toBeNull();

  findRecruiterContacts.mockRejectedValueOnce(new ApiError(503, 'recruiter contacts are unavailable right now'));
  await fireEvent.click(screen.getByRole('button', { name: 'Show recruiter contacts' }));
  expect(
    await screen.findByText("Couldn't look up contacts right now. Nothing was charged, try again later."),
  ).not.toBeNull();
});
