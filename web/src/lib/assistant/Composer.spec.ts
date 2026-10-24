import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Composer from './Composer.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/') },
}));

const { extractAttachment } = vi.hoisted(() => ({ extractAttachment: vi.fn() }));

vi.mock('#lib/assistant/client.js', () => ({ extractAttachment }));

beforeEach(() => {
  extractAttachment.mockReset();
});

function setup(props: Record<string, unknown> = {}) {
  const onSubmit = vi.fn();
  const view = render(Composer, {
    props: { queue: [], turnActive: false, disabled: false, onSubmit, onRemoveQueued: vi.fn(), ...props },
  });
  const input = view.container.querySelector('input[type="file"]') as HTMLInputElement;
  return { onSubmit, input, ...view };
}

async function choose(input: HTMLInputElement, file: File) {
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  await fireEvent.change(input);
}

const pdf = () => new File(['%PDF-1.7'], 'cv.pdf', { type: 'application/pdf' });

describe('Composer attachments', () => {
  it('reads a chosen PDF, shows it as a chip, and sends it with the message', async () => {
    extractAttachment.mockResolvedValue({ name: 'cv.pdf', text: 'Jane Doe', truncated: false });
    const { input, onSubmit } = setup();

    await choose(input, pdf());
    await screen.findByText('cv.pdf');

    // A document alone is a message: the send button is enabled with an empty draft.
    const send = screen.getByRole('button', { name: 'Send message' }) as HTMLButtonElement;
    expect(send.disabled).toBe(false);
    await fireEvent.click(send);

    expect(onSubmit).toHaveBeenCalledWith('', [{ name: 'cv.pdf', text: 'Jane Doe', truncated: false }]);
  });

  it('removes an attached file from the chip', async () => {
    extractAttachment.mockResolvedValue({ name: 'cv.pdf', text: 'Jane Doe', truncated: false });
    const { input } = setup();

    await choose(input, pdf());
    await fireEvent.click(await screen.findByRole('button', { name: 'Remove cv.pdf' }));

    expect(screen.queryByText('cv.pdf')).toBeNull();
    expect((screen.getByRole('button', { name: 'Send message' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('flags a document that was cut to fit', async () => {
    extractAttachment.mockResolvedValue({ name: 'book.pdf', text: 'x', truncated: true });
    const { input } = setup();

    await choose(input, pdf());

    expect(await screen.findByText('partial')).toBeTruthy();
  });

  it("shows the server's reason when a file cannot be read", async () => {
    extractAttachment.mockRejectedValue(new Error('it looks like a scan or image'));
    const { input } = setup();

    await choose(input, pdf());

    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('scan or image'));
    expect(screen.queryByRole('list', { name: 'Attached files' })).toBeNull();
  });
});

describe('Composer focus', () => {
  it('focus() puts the cursor in the message field', () => {
    const { component, container } = setup();

    component.focus();

    expect(document.activeElement).toBe(container.querySelector('textarea'));
  });
});

describe('Composer without attachments', () => {
  // A host that reads only the text (the AI search modal) must not offer a file it
  // would silently drop.
  it('offers no attach control when attachable is false', () => {
    const { container } = setup({ attachable: false });

    expect(screen.queryByRole('button', { name: /attach/i })).toBeNull();
    expect(container.querySelector('input[type="file"]')).toBeNull();
  });
});
