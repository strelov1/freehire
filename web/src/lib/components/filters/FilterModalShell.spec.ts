import { createRawSnippet } from 'svelte';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { RailEntry } from '#lib/filterSections.js';
import FilterModalShell from './FilterModalShell.svelte';

// The chrome every filter modal shares: the rail must say which section is open and
// what it has selected, the pane must be named by it, the footer must not render a
// blank count, and Clear all must reach a draft the wrapper keeps outside `staged`.

const rail: RailEntry[] = [
  { key: 'location', label: 'Location', section: 'ROLE', kind: 'location' },
  { key: 'salary', label: 'Salary', section: 'PAY & BENEFITS', kind: 'salary' },
];

const pane = createRawSnippet((entry: () => RailEntry) => ({
  render: () => `<p>${entry().label} controls</p>`,
}));

function setup(props: Record<string, unknown> = {}) {
  const staged = { active: 0, params: () => new URLSearchParams(), clear: vi.fn() };
  const onClose = vi.fn();
  render(FilterModalShell, {
    props: {
      open: true,
      onClose,
      rail,
      sections: ['ROLE', 'PAY & BENEFITS'],
      staged,
      entryCount: (e: RailEntry) => (e.key === 'location' ? 3 : 0),
      seed: () => {},
      apply: () => {},
      pane,
      ...props,
    },
  });
  return { staged, onClose };
}

describe('FilterModalShell', () => {
  it('marks the open section and names the pane after it', async () => {
    setup();
    const location = screen.getByRole('button', { name: /^Location\s?,\s?3 selected$/ });
    expect(location.getAttribute('aria-current')).toBe('true');
    expect(screen.getByRole('region', { name: 'Location' })).toBeTruthy();

    await fireEvent.click(screen.getByRole('button', { name: 'Salary' }));
    expect(location.getAttribute('aria-current')).toBeNull();
    expect(screen.getByRole('button', { name: 'Salary' }).getAttribute('aria-current')).toBe('true');
    expect(screen.getByRole('region', { name: 'Salary' })).toBeTruthy();
  });

  it('falls back to "Show jobs" when the count request fails', async () => {
    setup({ countsFetch: () => Promise.reject(new Error('down')) });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Show jobs' })).toBeTruthy());
  });

  it('announces a settled count politely', async () => {
    setup({ countsFetch: () => Promise.resolve({ total: 1234, facets: {} }) });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Show 1,234 jobs' })).toBeTruthy());
    expect(screen.getByText('1,234 jobs match').getAttribute('aria-live')).toBe('polite');
  });

  it('runs onClear after resetting the staged copy', async () => {
    const onClear = vi.fn();
    const { staged } = setup({ onClear });
    await fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    expect(staged.clear).toHaveBeenCalledOnce();
    expect(onClear).toHaveBeenCalledOnce();
  });

  it('leaves an Escape another handler claimed', async () => {
    const { onClose } = setup();
    const claimed = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
    claimed.preventDefault();
    window.dispatchEvent(claimed);
    expect(onClose).not.toHaveBeenCalled();
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('lets an open tooltip inside the dialog take the Escape', async () => {
    const withTooltip = createRawSnippet(() => ({ render: () => `<span role="tooltip">Hint</span>` }));
    const { onClose } = setup({ pane: withTooltip });
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
  });
});
