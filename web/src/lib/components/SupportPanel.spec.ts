import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import SupportPanel from './SupportPanel.svelte';

vi.mock('#lib/i18n/currentLocale.svelte.js', () => ({ locale: () => 'en' }));
vi.mock('#lib/assistant/AssistantChat.svelte', async () => {
  const stub = await import('../../../vitest-stubs/TurnStateChat.svelte');
  return { default: stub.default };
});

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
});
