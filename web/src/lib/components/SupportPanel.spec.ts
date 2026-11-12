import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import SupportPanel from './SupportPanel.svelte';

vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/assistant/AssistantChat.svelte', async () => {
  const stub = await import('../../../vitest-stubs/TurnStateChat.svelte');
  return { default: stub.default };
});

afterEach(() => vi.unstubAllGlobals());

describe('SupportPanel', () => {
  // Starting over remounts the chat, which would cut off a reply still streaming and
  // leave no way back to that conversation, so the control waits for the reply.
  it('disables New conversation while a reply streams', async () => {
    render(SupportPanel, { props: { open: true, onClose: () => {} } });
    const newChat = screen.getByRole('button', { name: 'New conversation' });
    expect((newChat as HTMLButtonElement).disabled).toBe(false);

    await fireEvent.click(screen.getByRole('button', { name: 'stub: reply starts' }));
    expect((newChat as HTMLButtonElement).disabled).toBe(true);

    await fireEvent.click(screen.getByRole('button', { name: 'stub: reply ends' }));
    expect((newChat as HTMLButtonElement).disabled).toBe(false);
  });

  it('asks whether the conversation helped only after a reply, and records the verdict', async () => {
    const fetcher = vi.fn((_url: string, _init?: RequestInit) => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal('fetch', fetcher);
    render(SupportPanel, { props: { open: true, onClose: () => {} } });

    expect(screen.queryByRole('button', { name: 'No, it did not help' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'stub: reply answered' }));

    await fireEvent.click(screen.getByRole('button', { name: 'No, it did not help' }));
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/support/sessions/support-1/rating',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ helpful: false }) }),
    );
    expect((await screen.findByRole('status')).textContent).toBe('Thanks for telling us.');
  });
});
