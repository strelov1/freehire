import { render, screen, cleanup } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import SourceBadge from './SourceBadge.svelte';

vi.mock('#lib/sourceKinds.js', () => ({
  loadSourceKinds: async () =>
    new Map([
      ['greenhouse', 'ats'],
      ['whatjobs', 'aggregator'],
      ['apple', 'company'],
      ['telegram', 'social'],
      ['manual', 'other'],
    ]),
}));
afterEach(cleanup);

it.each([
  ['greenhouse', 'Greenhouse', 'ATS'],
  ['whatjobs-uk', 'WhatJobs', 'Aggregator'],
  ['apple', 'Apple', 'Company'],
  ['telegram', 'Telegram', 'Community'],
  ['manual', 'Manual', 'Other'],
])('shows the brand and authoritative category for %s', async (source, brand, kind) => {
  render(SourceBadge, { source });
  expect(screen.getByText(brand)).toBeTruthy();
  expect(await screen.findByText(kind)).toBeTruthy();
});

it('keeps an unknown source visible without guessing its category', () => {
  render(SourceBadge, { source: 'new-board' });
  expect(screen.getByText('New Board')).toBeTruthy();
  expect(screen.queryByText('ATS')).toBeNull();
});
