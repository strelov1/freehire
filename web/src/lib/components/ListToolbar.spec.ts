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
});
