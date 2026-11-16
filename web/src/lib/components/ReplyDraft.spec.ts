import { fireEvent, render, screen } from '@testing-library/svelte';
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

it("shows the server's refusal and offers plans only when it carries an upgrade link", async () => {
  draftEmailReply.mockRejectedValueOnce(
    new ApiError(402, "Your plan doesn't include reply drafts.", { upgrade_url: '/pricing' }),
  );
  draftEmailReply.mockRejectedValueOnce(new ApiError(402, "You've used today's reply drafts."));
  render(ReplyDraft, { emailId: 42 });

  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  await screen.findByText("Your plan doesn't include reply drafts.");
  expect(screen.getByRole('link', { name: 'View plans' })).not.toBeNull();
  expect(screen.queryByRole('button', { name: 'Redraft' })).toBeNull();

  await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  await screen.findByText("You've used today's reply drafts.");
  expect(screen.queryByRole('link', { name: 'View plans' })).toBeNull();
});

it('ignores a slower draft from an earlier opening', async () => {
  let resolveFirst: (d: unknown) => void = () => {};
  draftEmailReply.mockReturnValueOnce(new Promise((r) => (resolveFirst = r)));
  draftEmailReply.mockResolvedValueOnce({ recipient: 'a@b.io', recipient_name: '', subject: 'Re: x', body: 'second' });
  render(ReplyDraft, { emailId: 42 });

  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Draft reply' }));
  const box = (await screen.findByLabelText('Reply')) as HTMLTextAreaElement;
  resolveFirst({ recipient: 'a@b.io', recipient_name: '', subject: 'Re: x', body: 'first' });
  await new Promise((r) => setTimeout(r, 0));
  expect(box.value).toBe('second');
});
