import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { Allowance, MatchAnalysisResponse } from '$lib/types';
import MatchSummary from './MatchSummary.svelte';

// MatchSummary pulls in SvelteKit runtime modules this test environment doesn't provide
// (no sveltekit() plugin in the `components` vitest project — see vitest.config.ts).
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

// The block reports; it does not act. `Tailor my CV` moved to the page's CTA row, so
// anything this component still fetched or still said about the tailoring allowance would
// be describing a button a screen above it — see
// openspec/changes/move-tailor-cv-to-job-cta. `api` is mocked to a getter that throws
// rather than to a vi.fn(): a spy would let a surviving call pass this file and fail
// somewhere else, while this names the component that made it.
vi.mock('$lib/api', () => ({
  get api(): never {
    throw new Error('MatchSummary must not call the API — the page passes the analysis in');
  },
}));

const spent: Allowance = {
  feature: 'tailor',
  limit: 3,
  used: 3,
  unlimited: false,
  enforced: true,
  resets_at: '2026-09-24T00:00:00Z',
};

const analysed: MatchAnalysisResponse = {
  has_cv: true,
  stale: false,
  analysis: {
    dimensions: [],
    requirement_match: [],
    hidden_signals: [],
    overall_score: 74,
    verdict: 'Good fit',
    strengths: [],
    gaps: ['No Kubernetes experience'],
    recommendation: '',
    blockers: [],
  },
};

describe('MatchSummary', () => {
  it('renders the cached analysis as a card linking to the full analysis', () => {
    render(MatchSummary, { props: { slug: 'rust-job', matchAnalysis: analysed } });

    expect(screen.getByText('74%')).toBeTruthy();
    expect(screen.getByText('Good fit')).toBeTruthy();
    expect(screen.getByText(/No Kubernetes experience/)).toBeTruthy();
    expect(screen.getByText(/view full analysis/i)).toBeTruthy();
  });

  it('prompts for a CV when the page has read that there is none', () => {
    render(MatchSummary, {
      props: { slug: 'rust-job', matchAnalysis: { has_cv: false, stale: false, analysis: null } },
    });

    expect(screen.getByText(/upload a cv to analyse/i)).toBeTruthy();
    expect(screen.getByRole('link', { name: /upload cv/i })).toBeTruthy();
  });

  it('offers no tailoring button, in any state', () => {
    for (const matchAnalysis of [null, analysed, { has_cv: true, stale: false, analysis: null }]) {
      const { unmount } = render(MatchSummary, { props: { slug: 'rust-job', matchAnalysis } });
      expect(screen.queryByRole('button', { name: /tailor/i })).toBeNull();
      unmount();
    }
  });

  // Both of these are said by ConfirmTailorDialog, at the moment the reader commits. A count
  // stated in two places is a count that can disagree.
  it('states nothing about the tailoring allowance', () => {
    render(MatchSummary, {
      props: {
        slug: 'rust-job',
        matchAnalysis: { has_cv: true, stale: false, analysis: null, tailor_allowance: spent },
      },
    });

    expect(screen.queryByText(/tailorings/i)).toBeNull();
    expect(screen.queryByText(/used today/i)).toBeNull();
    expect(screen.queryByText(/doesn't include/i)).toBeNull();
  });
});
