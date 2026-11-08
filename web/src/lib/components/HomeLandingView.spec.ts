import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { CatalogScale } from '#lib/types.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), afterNavigate: vi.fn(), beforeNavigate: vi.fn(), onNavigate: vi.fn() }));
vi.mock('#lib/api.js', () => ({
  api: {
    facetCounts: vi.fn(() => new Promise(() => {})),
    recentJobs: vi.fn(() => new Promise(() => {})),
    searchJobs: vi.fn(() => new Promise(() => {})),
  },
}));
// The live feed opens an SSE stream; it is not what these tests are about.
vi.mock('./RecentJobsFeed.svelte', () => ({ default: () => {} }));
vi.stubGlobal('IntersectionObserver', class { observe() {} unobserve() {} disconnect() {} });

import HomeLandingView from './HomeLandingView.svelte';

const scale = { unique_open_jobs: 5_212_340, companies: 700_009, sources: 437, ats_platforms: 178 } as CatalogScale;

describe('HomeLandingView hero', () => {
  it('quotes the live catalogue size, compacted', () => {
    render(HomeLandingView, { props: { counts: null, scale } });
    expect(screen.getByText(/5\.2M\+ live postings/)).toBeTruthy();
  });

  it('floors rather than rounds, so the "+" never overstates', () => {
    render(HomeLandingView, { props: { counts: null, scale: { ...scale, unique_open_jobs: 4_968_000 } } });
    expect(screen.getByText(/4\.9M\+ live postings/)).toBeTruthy();
  });

  // A cold snapshot must not make the hero say nothing, or overstate: the fallback is a floor.
  it('falls back to the last measured floor when the snapshot is missing', () => {
    render(HomeLandingView, { props: { counts: null, scale: null } });
    expect(screen.getByText(/4\.9M\+ live postings/)).toBeTruthy();
  });

  it('links the agent and API doors from the hero sentence', () => {
    render(HomeLandingView, { props: { counts: null, scale: null } });
    expect(screen.getByRole('link', { name: 'your AI agent' }).getAttribute('href')).toBe('/agents');
    expect(screen.getByRole('link', { name: 'the API' }).getAttribute('href')).toBe('/jobs-api');
  });
});
