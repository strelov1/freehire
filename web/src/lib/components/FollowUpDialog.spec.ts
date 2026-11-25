import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '#lib/api.js';
import { clipboardText } from '#lib/followup.js';
import type { FollowUpDraft } from '#lib/types.js';
import FollowUpDialog from './FollowUpDialog.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/jobs') } }));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('#lib/api.js', () => ({
  api: {
    getFollowUpDraft: vi.fn(),
    recordFollowUp: vi.fn(),
  },
}));

const DRAFT: FollowUpDraft = {
  recipient: 'talent@acme.test',
  recipient_name: 'Acme Talent',
  subject: 'Following up on the Senior Product Engineer role',
  body: 'Hi Acme,\n\nI wanted to follow up on my application.',
  days_silent: 9,
};

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

// jsdom has no clipboard; without it `copy()` reports copyFailed and never records, which
// is the component being right about an environment that is wrong.
const writeText = vi.fn(async (_text: string) => undefined);

beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  writeText.mockReset().mockResolvedValue(undefined);
  vi.mocked(api.getFollowUpDraft).mockReset().mockResolvedValue(DRAFT);
  vi.mocked(api.recordFollowUp).mockReset().mockResolvedValue(undefined);
});

describe('FollowUpDialog', () => {
  it('names the dialog by its heading and describes it with the company and the silence', async () => {
    render(FollowUpDialog, {
      slug: 'acme-senior-product-engineer',
      company: 'Acme',
      onclose: vi.fn(),
      onrecorded: vi.fn(),
    });

    await waitFor(() => expect(dialog().textContent).toContain(DRAFT.subject));
    expect((dialog() as HTMLDialogElement).open).toBe(true);
    expect(within(dialog()).getByRole('heading', { level: 2, name: 'Follow up' })).toBeTruthy();
    const describedBy = must(dialog().getAttribute('aria-describedby'), 'aria-describedby');
    expect(document.getElementById(describedBy)?.textContent).toBe('Acme — no reply for 9 days');
  });

  it('hands the draft to the candidate and records the chase, by either compose link or Copy', async () => {
    for (const take of [
      function gmailLink() {
        return fireEvent.click(within(dialog()).getByRole('link', { name: 'Gmail' }));
      },
      function mailAppLink() {
        return fireEvent.click(within(dialog()).getByRole('link', { name: 'Mail app' }));
      },
      function copyButton() {
        return fireEvent.click(within(dialog()).getByRole('button', { name: 'Copy draft' }));
      },
    ]) {
      cleanup();
      const onrecorded = vi.fn();
      const onclose = vi.fn();
      render(FollowUpDialog, { slug: 'acme-senior-product-engineer', company: 'Acme', onclose, onrecorded });
      // The links only exist once the draft has arrived.
      await waitFor(() => expect(within(dialog()).getByRole('link', { name: 'Gmail' })).toBeTruthy());

      await take();

      expect(api.recordFollowUp).toHaveBeenCalledWith('acme-senior-product-engineer');
      // `onrecorded` fires once the POST resolves, not in the click's own flush.
      await waitFor(() => expect(onrecorded).toHaveBeenCalledTimes(1));
      if (take.name === 'copyButton') expect(writeText).toHaveBeenCalledWith(clipboardText(DRAFT));
      // Taking the draft is not a way out: the candidate may still want the other link.
      // The flush is what a close would land in, so the check waits for it rather than
      // reading the mock one tick too early.
      await Promise.resolve();
      expect(onclose).not.toHaveBeenCalled();
      expect((dialog() as HTMLDialogElement).open).toBe(true);
    }
  });

  it('reports the header X, the footer Close and Escape through onclose', async () => {
    // The two buttons share the name "Close" and do the same thing, so each is checked.
    const ways = ['x', 'close', 'escape'] as const;
    for (const way of ways) {
      cleanup();
      const onclose = vi.fn();
      render(FollowUpDialog, { slug: 'acme-senior-product-engineer', company: 'Acme', onclose, onrecorded: vi.fn() });

      if (way === 'escape') {
        // Escape: the browser fires `cancel`, then closes the dialog, which fires `close`.
        // jsdom implements neither default action, so both halves are dispatched by hand;
        // Dialog listens for `cancel` only to be able to refuse it, and to `close` to react.
        fireEvent(dialog(), new Event('cancel', { cancelable: true }));
        fireEvent(dialog(), new Event('close'));
        await Promise.resolve();
      } else {
        const [x, footerClose] = within(dialog()).getAllByRole('button', { name: 'Close' });
        expect(within(dialog()).getAllByRole('button', { name: 'Close' })).toHaveLength(2);
        await fireEvent.click(must(way === 'x' ? x : footerClose, 'close button'));
      }

      expect(onclose, way).toHaveBeenCalledTimes(1);
    }
  });
});
