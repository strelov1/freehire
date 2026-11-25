import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import GmailConnectDialog from './GmailConnectDialog.svelte';

// The dialog is mounted by its parent to open it and reports every way of leaving through
// onClose, so that is the whole contract: two ways out that only close, and one that proceeds.

function must<T>(v: T | null | undefined, what: string): T {
  if (v == null) throw new Error(`missing ${what}`);
  return v;
}

function dialog(): HTMLElement {
  return must(
    screen.getAllByRole('dialog', { hidden: true }).find((d) => (d as HTMLDialogElement).open),
    'open dialog',
  );
}

describe('GmailConnectDialog', () => {
  it('is opened as a modal and named by its title, described by its intro', () => {
    render(GmailConnectDialog, { onClose: vi.fn(), onConnect: vi.fn() });

    // showModal(), not the `open` attribute: the top layer, the inert background and the
    // focus trap all come from it. `aria-modal` is left to the browser for the same reason.
    expect((dialog() as HTMLDialogElement).open).toBe(true);
    expect(within(dialog()).getByRole('heading', { level: 2, name: 'Connect Gmail' })).toBeTruthy();
    const describedBy = must(dialog().getAttribute('aria-describedby'), 'aria-describedby');
    expect(document.getElementById(describedBy)?.textContent).toBe(
      'Turn recruiting emails into application updates, without sorting them by hand.',
    );
  });

  it('reports the close button, Cancel and Escape through onClose, without connecting', async () => {
    for (const leave of [
      () => fireEvent.click(within(dialog()).getByRole('button', { name: 'Close' })),
      () => fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel' })),
      // Escape: the browser fires `cancel`, then closes the dialog, which fires `close`.
      // jsdom implements neither default action, so both halves are dispatched by hand;
      // Dialog listens for `cancel` only to be able to refuse it, and to `close` to react.
      async () => {
        fireEvent(dialog(), new Event('cancel', { cancelable: true }));
        fireEvent(dialog(), new Event('close'));
      },
    ]) {
      cleanup();
      const onClose = vi.fn();
      const onConnect = vi.fn();
      render(GmailConnectDialog, { onClose, onConnect });

      await leave();

      expect(onClose).toHaveBeenCalledTimes(1);
      expect(onConnect).not.toHaveBeenCalled();
    }
  });

  it('proceeds to Google on Connect Gmail and leaves the dialog open', async () => {
    const onClose = vi.fn();
    const onConnect = vi.fn();
    render(GmailConnectDialog, { onClose, onConnect });

    await fireEvent.click(within(dialog()).getByRole('button', { name: 'Connect Gmail' }));

    expect(onConnect).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    expect((dialog() as HTMLDialogElement).open).toBe(true);
  });
});
