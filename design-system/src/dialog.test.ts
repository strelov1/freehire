import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import Dialog from './dialog.svelte';
import { must } from './test-utils';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

const open = (open: boolean) => ({ open, children: slot('<p>body</p>') });

describe('Dialog', () => {
  beforeEach(() => {
    document.body.style.overflow = 'scroll';
  });

  it('locks the page while open and restores what was there before', async () => {
    const dialog = render(Dialog, open(true));
    expect(document.body.style.overflow).toBe('hidden');

    await dialog.rerender(open(false));
    expect(document.body.style.overflow).toBe('scroll');
  });

  it('stays locked until the last of two nested dialogs closes', async () => {
    const outer = render(Dialog, open(true));
    const inner = render(Dialog, open(true));
    expect(document.body.style.overflow).toBe('hidden');

    await inner.rerender(open(false));
    expect(document.body.style.overflow).toBe('hidden');

    await outer.rerender(open(false));
    expect(document.body.style.overflow).toBe('scroll');
  });

  // The regression, and the order that exposes it: each dialog used to capture
  // body overflow for itself, so the outer one restored "scroll" while the
  // inner was still open, and the inner then restored the "hidden" it had read
  // from the outer — locking the page with nothing left on screen. Closing
  // strictly innermost-first happened to paper over both halves of that.
  it('survives nested dialogs closing in the order they opened', async () => {
    const outer = render(Dialog, open(true));
    const inner = render(Dialog, open(true));

    await outer.rerender(open(false));
    expect(document.body.style.overflow).toBe('hidden');

    await inner.rerender(open(false));
    expect(document.body.style.overflow).toBe('scroll');
  });

  it('releases the lock when an open dialog is unmounted', () => {
    const dialog = render(Dialog, open(true));
    expect(document.body.style.overflow).toBe('hidden');

    dialog.unmount();
    expect(document.body.style.overflow).toBe('scroll');
  });

  it('names itself from the title and description it was given', () => {
    const { getByRole } = render(Dialog, {
      ...open(true),
      title: 'Delete job',
      description: 'This cannot be undone.',
    });

    const el = getByRole('dialog', { hidden: true });
    const labelledBy = el.getAttribute('aria-labelledby');
    const describedBy = el.getAttribute('aria-describedby');
    expect(labelledBy).toBeTruthy();
    expect(describedBy).toBeTruthy();
    expect(document.getElementById(must(labelledBy))?.textContent).toBe('Delete job');
    expect(document.getElementById(must(describedBy))?.textContent).toBe('This cannot be undone.');
  });

  it('carries no name when it was given no title', () => {
    const { getByRole } = render(Dialog, open(true));

    expect(getByRole('dialog', { hidden: true }).getAttribute('aria-labelledby')).toBeNull();
  });

  // A bare `max-w-*` override used to share twMerge's conflict group with the
  // base's unprefixed `max-w-none`, so the caller's class won at every width
  // and cancelled the mobile takeover below `sm`. An `sm:`-prefixed override
  // lives in its own group and must leave `max-w-none` in place.
  it('keeps the mobile takeover when the caller sizes the card at sm and up', () => {
    const { getByRole } = render(Dialog, { ...open(true), class: 'sm:max-w-md' });
    const el = getByRole('dialog', { hidden: true });

    expect(el.className).toContain('max-w-none');
    expect(el.className).toContain('sm:max-w-md');
  });

  // A dialog holding the outcome of a request that cannot be repeated has to be
  // able to refuse to go away — otherwise Escape hides whether the irreversible
  // thing succeeded. The platform gives us the refusal for free through
  // `cancel`, but only if someone calls preventDefault on it.
  describe('when not dismissible', () => {
    const undismissable = () => ({ ...open(true), dismissible: false });

    it('refuses the platform’s own cancel', () => {
      const { getByRole } = render(Dialog, undismissable());
      const el = getByRole('dialog', { hidden: true });

      const cancel = new Event('cancel', { cancelable: true });
      el.dispatchEvent(cancel);

      expect(cancel.defaultPrevented).toBe(true);
    });

    it('ignores a click on its backdrop', async () => {
      const { getByRole } = render(Dialog, undismissable());
      const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

      el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();

      expect(el.open).toBe(true);
    });

    // Preventing `cancel` is not enough on its own. The platform's close watcher
    // spends a user-activation budget on each preventDefault, and once that runs
    // out it fires `cancel` unprevented and closes anyway — measured in Chrome:
    // two Escapes refused, the third closes. It is a deliberate anti-trap valve,
    // so the only way to honour the contract is to reassert the dialog.
    it('reopens itself if the platform closes it anyway', async () => {
      const { getByRole } = render(Dialog, undismissable());
      const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

      el.close();
      await Promise.resolve();

      expect(el.open).toBe(true);
    });

    it('offers no close button', () => {
      const { queryByLabelText } = render(Dialog, undismissable());

      expect(queryByLabelText('Close')).toBeNull();
    });
  });

  it('closes on its backdrop and offers a close button by default', async () => {
    const { getByRole, getByLabelText } = render(Dialog, open(true));
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

    expect(getByLabelText('Close')).toBeTruthy();

    el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await Promise.resolve();

    expect(el.open).toBe(false);
  });
});
