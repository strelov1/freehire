import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import Dialog from './dialog.svelte';

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
    expect(el.getAttribute('aria-labelledby')).toBeTruthy();
    expect(document.getElementById(el.getAttribute('aria-labelledby')!)?.textContent).toBe(
      'Delete job',
    );
    expect(document.getElementById(el.getAttribute('aria-describedby')!)?.textContent).toBe(
      'This cannot be undone.',
    );
  });

  it('carries no name when it was given no title', () => {
    const { getByRole } = render(Dialog, open(true));

    expect(getByRole('dialog', { hidden: true }).getAttribute('aria-labelledby')).toBeNull();
  });
});
