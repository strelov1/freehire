import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import FilterSummaryShell, { type SummaryGroup } from './FilterSummaryShell.svelte';
import Harness from './FilterSummaryShellTestHarness.svelte';

describe('FilterSummaryShell', () => {
  it('names an excluded chip as excluded', () => {
    const groups: SummaryGroup[] = [
      {
        label: 'Skills',
        chips: [
          { key: 'skills:go', text: 'Go', exclude: false, remove: () => {} },
          { key: 'skills:php', text: 'PHP', exclude: true, remove: () => {} },
        ],
      },
    ];
    render(FilterSummaryShell, { groups, active: 2, onReset: () => {}, onOpen: () => {} });
    expect(screen.getByRole('button', { name: 'Remove Go' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Remove excluded PHP' })).toBeTruthy();
    expect(screen.getByText('Excluded:')).toBeTruthy();
  });

  it('moves focus to the next chip when a chip removes itself', async () => {
    render(Harness, { values: ['Go', 'Rust', 'Zig'] });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Rust' }));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Remove Zig' }));
  });

  it('falls back to the previous chip when the last one goes', async () => {
    render(Harness, { values: ['Go', 'Rust'] });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Rust' }));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Remove Go' }));
  });

  it('moves focus to the heading after Clear all', async () => {
    render(Harness, { values: ['Go', 'Rust'] });
    await fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Filters' }));
  });

  it('moves focus to the heading once a group clear leaves no chip', async () => {
    render(Harness, { values: ['Go', 'Rust'] });
    await fireEvent.click(screen.getByRole('button', { name: 'Clear Skills' }));
    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Filters' }));
  });
});
