import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const { draftEmailReply } = vi.hoisted(() => ({ draftEmailReply: vi.fn() }));
vi.mock('#lib/api.js', () => ({ api: { draftEmailReply } }));
vi.mock('#lib/announce.svelte.js', () => ({ announce: vi.fn() }));
vi.mock('#lib/i18n/currentLocale.svelte', () => ({ locale: () => 'en' }));

import ReplyDraft from './ReplyDraft.svelte';

beforeEach(() => {
  draftEmailReply.mockReset();
});

it('drafts on click and hands the edited text to the copy and compose links', async () => {
  draftEmailReply.mockResolvedValue({
    recipient: 'anna@acme.io',
    recipient_name: 'Anna',
    subject: 'Re: Interview',
    body: 'Hi Anna, [times that suit you]',
  });
  const copied = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copied } });
  render(ReplyDraft, { emailId: 42 });

  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  const box = await screen.findByLabelText('Reply');
  expect(draftEmailReply).toHaveBeenCalledWith(42);

  await fireEvent.input(box, { target: { value: 'Hi Anna, Tuesday works.' } });
  await fireEvent.click(screen.getByRole('button', { name: /Copy reply/ }));
  expect(copied).toHaveBeenCalledWith('Hi Anna, Tuesday works.');
  const gmail = screen.getByRole('link', { name: 'Gmail' }).getAttribute('href') ?? '';
  expect(gmail).toContain('to=anna%40acme.io');
  expect(gmail).toContain(encodeURIComponent('Tuesday works.'));
});

it('offers the plans instead of an error to a free account', async () => {
  draftEmailReply.mockRejectedValue(new ApiError(402, 'payment required'));
  render(ReplyDraft, { emailId: 42 });

  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  await waitFor(() => expect(screen.getByText('Reply drafts are part of Pro.')).not.toBeNull());
  expect(screen.getByRole('link', { name: 'View plans' })).not.toBeNull();
  expect(screen.queryByRole('button', { name: 'Redraft' })).toBeNull();
});
