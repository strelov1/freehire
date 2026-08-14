import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import ConfirmDialog from './confirm-dialog.svelte';

function deferred<T = void>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('ConfirmDialog', () => {
  it('calls onConfirm and closes once it resolves', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    const { getByRole } = render(ConfirmDialog, {
      open: true,
      title: 'Delete saved search?',
      onConfirm,
    });
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

    await fireEvent.click(getByRole('button', { name: 'Confirm' }));
    expect(onConfirm).toHaveBeenCalledOnce();

    await vi.waitFor(() => expect(el.open).toBe(false));
  });

  it('leaves cancel a no-op on onConfirm and just closes', async () => {
    const onConfirm = vi.fn();
    const { getByRole } = render(ConfirmDialog, {
      open: true,
      title: 'Delete saved search?',
      onConfirm,
    });
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

    await fireEvent.click(getByRole('button', { name: 'Cancel' }));

    expect(onConfirm).not.toHaveBeenCalled();
    expect(el.open).toBe(false);
  });

  it('holds itself open and non-dismissible while onConfirm is pending', async () => {
    const { promise, resolve } = deferred();
    const onConfirm = vi.fn().mockReturnValue(promise);
    const { getByRole, queryByLabelText } = render(ConfirmDialog, {
      open: true,
      title: 'Delete saved search?',
      onConfirm,
    });

    await fireEvent.click(getByRole('button', { name: 'Confirm' }));

    expect((getByRole('button', { name: /Confirm/ }) as HTMLButtonElement).disabled).toBe(true);
    expect((getByRole('button', { name: 'Cancel' }) as HTMLButtonElement).disabled).toBe(true);
    // Dialog only omits its own close button while dismissible is false.
    expect(queryByLabelText('Close')).toBeNull();

    resolve();
  });

  it('stays open and shows the thrown message when onConfirm rejects', async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error('Could not delete. Please try again.'));
    const { getByRole, findByText } = render(ConfirmDialog, {
      open: true,
      title: 'Delete saved search?',
      onConfirm,
    });
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

    await fireEvent.click(getByRole('button', { name: 'Confirm' }));

    expect(await findByText('Could not delete. Please try again.')).toBeTruthy();
    expect(el.open).toBe(true);
    expect((getByRole('button', { name: 'Confirm' }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('clears a previous error once reopened', async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error('Could not delete.'));
    const { getByRole, findByText, queryByText, rerender } = render(ConfirmDialog, {
      open: true,
      title: 'Delete saved search?',
      onConfirm,
    });
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;

    await fireEvent.click(getByRole('button', { name: 'Confirm' }));
    await findByText('Could not delete.');

    await rerender({ open: false, title: 'Delete saved search?', onConfirm });
    await rerender({ open: true, title: 'Delete saved search?', onConfirm });

    expect(queryByText('Could not delete.')).toBeNull();
    expect(el.open).toBe(true);
  });
});
