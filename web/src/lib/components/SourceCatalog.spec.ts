import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import type { SourceEntry } from '$lib/types';
import SourceCatalog from './SourceCatalog.svelte';

function entry(over: Partial<SourceEntry> & { source: string }): SourceEntry {
  return {
    kind: 'ats',
    logo_host: null,
    jobs: null,
    health: null,
    ...over,
  };
}

const greenhouse = entry({
  source: 'greenhouse',
  kind: 'ats',
  logo_host: 'job-boards.greenhouse.io',
  jobs: { open: 1200, browsable: 900, measured_at: '2026-09-16T04:00:00Z' },
  health: {
    status: 'operational',
    total_boards: 900,
    healthy_boards: 900,
    cooled_boards: 0,
    last_run: '2026-09-16T05:00:00Z',
    last_success: '2026-09-16T05:00:00Z',
    ingested_total: 4321,
  },
});

const adzuna = entry({
  source: 'adzuna',
  kind: 'aggregator',
  logo_host: 'www.adzuna.com',
  jobs: { open: 100, browsable: 40, ats_matched: 70, ats_unmatched: 30, measured_at: '2026-09-16T04:00:00Z' },
  health: null,
});

describe('SourceCatalog', () => {
  it('links a source to its own filtered search, using the de-duplicated count', () => {
    render(SourceCatalog, { sources: [greenhouse] });

    // The number on the card and the search it opens must agree, so the label is the
    // de-duplicated 900 and never the raw 1,200.
    const link = screen.getByRole('link', { name: /900 jobs/ });
    expect(link.getAttribute('href')).toBe('/jobs?source=greenhouse');
    expect(screen.queryByText(/1,200/)).toBeNull();
  });

  it('never calls unmatched postings exclusive', () => {
    const { container } = render(SourceCatalog, { sources: [adzuna] });

    // The dedup pass finding no pair is the absence of evidence. The word would convert
    // that into a claim, and the claim would be quoted back at us.
    expect(container.textContent?.toLowerCase()).not.toContain('exclusive');
    expect(container.innerHTML.toLowerCase()).not.toContain('exclusive');
    expect(screen.getByText(/Of 100 postings before de-duplication/)).toBeTruthy();
    expect(screen.getByText(/70% also found on an ATS/)).toBeTruthy();
    expect(screen.getByText(/30% not matched to one/)).toBeTruthy();
  });

  it('shows no overlap figures for a source that is not an aggregator', () => {
    render(SourceCatalog, { sources: [greenhouse] });

    expect(screen.queryByText(/found on an ATS/)).toBeNull();
  });

  it('never folds an unmeasured count into the headline total as a zero', () => {
    // One unreachable Meilisearch leaves every `browsable` null. A headline reading
    // "0 jobs indexed" above cards that all say "not measured yet" is the exact
    // substitution the whole feature exists to prevent — and it is one `?? 0` away.
    const { container } = render(SourceCatalog, {
      sources: [entry({ source: 'greenhouse', jobs: null }), entry({ source: 'lever', jobs: null })],
    });

    expect(container.textContent).not.toContain('0 jobs indexed');
    expect(screen.getByText(/job counts not measured yet/)).toBeTruthy();
  });

  it('says how many sources the headline total actually covers', () => {
    render(SourceCatalog, { sources: [greenhouse, entry({ source: 'lever', jobs: null })] });

    expect(screen.getByText(/900 jobs indexed across 1 of them/)).toBeTruthy();
  });

  it('states when the counts were measured', () => {
    // The figures are a snapshot up to a few hours old. A snapshot that will not say when
    // it was taken is asking to be read as live.
    render(SourceCatalog, { sources: [greenhouse] });

    expect(screen.getByText(/counted/)).toBeTruthy();
  });

  it('never rounds a non-zero count into an absolute percentage', () => {
    // 1 posting in 100,000 is not "0% also found on an ATS", and its complement is not
    // "100% not matched to one" — the strongest form of the claim this feature refuses to
    // make in words, reached by rounding.
    render(SourceCatalog, {
      sources: [
        entry({
          source: 'adzuna',
          kind: 'aggregator',
          jobs: {
            open: 100000,
            browsable: 100000,
            ats_matched: 1,
            ats_unmatched: 99999,
            measured_at: '2026-09-16T04:00:00Z',
          },
        }),
      ],
    });

    expect(screen.getByText(/<1% also found on an ATS/)).toBeTruthy();
    expect(screen.getByText(/>99% not matched to one/)).toBeTruthy();
    expect(screen.queryByText(/100% not matched/)).toBeNull();
  });

  it('still says 100% when every posting really was matched', () => {
    render(SourceCatalog, {
      sources: [
        entry({
          source: 'adzuna',
          kind: 'aggregator',
          jobs: {
            open: 50,
            browsable: 0,
            ats_matched: 50,
            ats_unmatched: 0,
            measured_at: '2026-09-16T04:00:00Z',
          },
        }),
      ],
    });

    expect(screen.getByText(/100% also found on an ATS · 0% not matched to one/)).toBeTruthy();
  });

  it('does not call a registered adapter "not a crawl adapter"', () => {
    // `health === null` means two different things: a source that is not a crawl adapter
    // (telegram), and a registered adapter with no health record yet — which is the very
    // case the snapshot's union spine exists to cover. Conflating them puts "Greenhouse —
    // not a crawl adapter" on a public page.
    render(SourceCatalog, {
      sources: [
        entry({ source: 'greenhouse', kind: 'ats', health: null }),
        entry({ source: 'telegram', kind: 'other', health: null }),
      ],
    });

    expect(screen.getByText(/no crawl recorded yet/)).toBeTruthy();
    expect(screen.getByText(/not a crawl adapter/)).toBeTruthy();
  });

  it('tells an unmeasured count apart from a measured zero', () => {
    const unmeasured = entry({ source: 'lever', jobs: null });
    const empty = entry({
      source: 'ashby',
      jobs: { open: 0, browsable: 0, measured_at: '2026-09-16T04:00:00Z' },
    });

    render(SourceCatalog, { sources: [unmeasured, empty] });

    expect(screen.getByText(/not measured yet/)).toBeTruthy();
    expect(screen.getByText(/No open jobs right now/)).toBeTruthy();
  });

  it('filters in the browser and states a no-match plainly', async () => {
    render(SourceCatalog, { sources: [greenhouse, adzuna] });

    const box = screen.getByLabelText('Search sources');

    await fireEvent.input(box, { target: { value: 'adz' } });
    expect(screen.queryByText('Greenhouse')).toBeNull();
    expect(screen.getByText('Adzuna')).toBeTruthy();

    await fireEvent.input(box, { target: { value: 'nothing-matches-this' } });
    // An empty list under a group heading reads as a broken page; say it instead.
    expect(screen.getByText(/No source matches/)).toBeTruthy();
    expect(screen.queryByText('ATS platforms')).toBeNull();
  });

  it('loads logos lazily', () => {
    const { container } = render(SourceCatalog, { sources: [greenhouse] });

    // A few hundred eagerly-fetched logos would cost more than everything else on the page.
    const img = container.querySelector('img');
    expect(img?.getAttribute('loading')).toBe('lazy');
  });

  it('renders a placeholder rather than a broken image when there is no host', () => {
    const { container } = render(SourceCatalog, {
      sources: [entry({ source: 'telegram', kind: 'other', logo_host: null })],
    });

    expect(container.querySelector('img')).toBeNull();
  });

  it('renders an unavailable state rather than an empty page when the read failed', () => {
    render(SourceCatalog, { sources: null });

    expect(screen.getByText(/unavailable right now/)).toBeTruthy();
    expect(screen.queryByLabelText('Search sources')).toBeNull();
  });
});
