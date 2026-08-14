import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import NoticeDialog from './notice-dialog.svelte';

describe('NoticeDialog', () => {
  it('names itself from the title and closes on its one button', async () => {
    const { getByRole } = render(NoticeDialog, {
      open: true,
      title: 'Your changes were saved.',
    });
    const el = getByRole('dialog', { hidden: true }) as HTMLDialogElement;
    expect(el.open).toBe(true);

    await fireEvent.click(getByRole('button', { name: 'OK' }));

    expect(el.open).toBe(false);
  });

  it('uses a custom confirm label in place of the default OK', () => {
    const { getByRole } = render(NoticeDialog, {
      open: true,
      title: 'A new version is available',
      confirmLabel: 'Reload',
    });

    expect(getByRole('button', { name: 'Reload' })).toBeTruthy();
  });
});
