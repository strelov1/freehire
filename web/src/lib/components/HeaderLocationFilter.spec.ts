import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, it, vi } from 'vitest';
import type { FacetStore } from '#lib/facets.js';
import HeaderLocationFilter from './HeaderLocationFilter.svelte';

vi.mock('$app/navigation', () => ({ afterNavigate: vi.fn(), goto: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('./filters/LocationPane.svelte', () => {
  throw new Error('chunk unavailable');
});

it('lets visitors close and reopen location filters after a chunk fails', async () => {
  const store = { facet: () => ({ include: [], exclude: [] }) } as unknown as FacetStore;
  render(HeaderLocationFilter, { store });
  await fireEvent.click(screen.getByRole('button', { name: 'Location' }));
  expect((await screen.findByRole('alert')).textContent).toContain('Location filters could not load.');
  await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  expect(screen.queryByRole('dialog')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Location' }));
  expect(await screen.findByRole('alert')).toBeTruthy();
});
