import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

import type { CvBaseItem } from '#lib/cv.js';
import BasePicker from './BasePicker.svelte';

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/cvs') } }));

const base = (id: string, title: string, isDefault = false): CvBaseItem => ({
  id,
  title,
  template_id: 'classic-ats',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-02T10:00:00Z',
  is_default: isDefault,
  hand_edited: false,
});

describe('BasePicker', () => {
  it('renders nothing when there is no choice to make', () => {
    const { container: none } = render(BasePicker, { bases: [] });
    expect(none.querySelector('input')).toBeNull();
    const { container: one } = render(BasePicker, { bases: [base('a', 'Only')] });
    expect(one.querySelector('input')).toBeNull();
  });

  it('offers a radio per base, preselects the bound value and marks the default', () => {
    render(BasePicker, { bases: [base('a', 'Backend'), base('b', 'Data', true)], value: 'b' });
    const radios = screen.getAllByRole('radio') as HTMLInputElement[];
    expect(radios).toHaveLength(2);
    expect(radios.find((r) => r.value === 'b')?.checked).toBe(true);
    expect(screen.getByText('Default')).toBeTruthy();
  });

  it('lets the candidate pick another base', async () => {
    render(BasePicker, { bases: [base('a', 'Backend', true), base('b', 'Data')], value: 'a' });
    const second = screen.getAllByRole('radio').find((r) => (r as HTMLInputElement).value === 'b') as HTMLInputElement;
    await fireEvent.click(second);
    expect(second.checked).toBe(true);
  });
});
