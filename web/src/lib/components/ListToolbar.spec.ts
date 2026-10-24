import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import ListToolbar from './ListToolbar.svelte';

// The result count's live region must outlive the reload: a region inserted together
// with its text is not announced, which is what the old `{#if total !== null}` did.

describe('ListToolbar result announcement', () => {
  it('stays mounted and silent on first render', () => {
    render(ListToolbar, { total: 120, unit: 'jobs', settled: true });
    expect(screen.getByRole('status').textContent).toBe('');
  });

  it('announces the new count after a reload, and "no matching" on zero', async () => {
    const view = render(ListToolbar, { total: 120, unit: 'jobs', settled: true });
    const region = screen.getByRole('status');

    await view.rerender({ total: null, unit: 'jobs', settled: false });
    expect(screen.getByRole('status')).toBe(region);
    expect(region.textContent).toBe('');

    await view.rerender({ total: 7, unit: 'jobs', settled: true });
    expect(region.textContent).toBe('7 jobs');

    await view.rerender({ total: null, unit: 'jobs', settled: false });
    await view.rerender({ total: null, unit: 'jobs', settled: true });
    expect(region.textContent).toBe('No matching jobs');
  });

  it('re-announces an unchanged count after a reload', async () => {
    const view = render(ListToolbar, { total: 7, unit: 'jobs', settled: true });
    const region = screen.getByRole('status');
    await view.rerender({ total: null, unit: 'jobs', settled: false });
    await view.rerender({ total: 7, unit: 'jobs', settled: true });
    expect(region.textContent).toBe('7 jobs');
  });

  it('skips the first count for a caller that loads on the client', async () => {
    const view = render(ListToolbar, { total: null, unit: 'companies' });
    const region = screen.getByRole('status');
    await view.rerender({ total: 40, unit: 'companies' });
    expect(region.textContent).toBe('');
    await view.rerender({ total: null, unit: 'companies' });
    await view.rerender({ total: 12, unit: 'companies' });
    expect(region.textContent).toBe('12 companies');
  });

  it('keeps quiet across a page turn of the same result set, but speaks a new set or count', async () => {
    const view = render(ListToolbar, { total: 1234, unit: 'jobs', settled: true, announceKey: 'q=go' });
    const region = screen.getByRole('status');

    // Page 2 of the same query: reloads, lands on the same total, says nothing.
    await view.rerender({ total: null, unit: 'jobs', settled: false, announceKey: 'q=go' });
    await view.rerender({ total: 1234, unit: 'jobs', settled: true, announceKey: 'q=go' });
    expect(region.textContent).toBe('');

    // A changed query with the same total is still news.
    await view.rerender({ total: null, unit: 'jobs', settled: false, announceKey: 'q=rust' });
    await view.rerender({ total: 1234, unit: 'jobs', settled: true, announceKey: 'q=rust' });
    expect(region.textContent).toBe('1,234 jobs');

    // A page turn keeps the spoken text rather than clearing and re-speaking it.
    await view.rerender({ total: null, unit: 'jobs', settled: false, announceKey: 'q=rust' });
    expect(region.textContent).toBe('1,234 jobs');
    await view.rerender({ total: 1234, unit: 'jobs', settled: true, announceKey: 'q=rust' });
    expect(region.textContent).toBe('1,234 jobs');

    // The same query whose count moved is spoken.
    await view.rerender({ total: null, unit: 'jobs', settled: false, announceKey: 'q=rust' });
    await view.rerender({ total: 1240, unit: 'jobs', settled: true, announceKey: 'q=rust' });
    expect(region.textContent).toBe('1,240 jobs');
  });
});
