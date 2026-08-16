import { describe, expect, it } from 'vitest';

import { createApi } from './api';
import type { CatalogScale } from './types';

/** A fetch that records the URL it was asked for and answers with a fixed envelope. */
function snapshotFetch(seen: { url?: string }, snapshot: Partial<CatalogScale>): typeof fetch {
  return ((url: string) => {
    seen.url = url;
    return Promise.resolve(new Response(JSON.stringify({ data: snapshot }), { status: 200 }));
  }) as unknown as typeof fetch;
}

describe('catalogScale', () => {
  it('reads every scale figure from one request', async () => {
    const seen: { url?: string } = {};
    const client = createApi(
      snapshotFetch(seen, {
        open_jobs: 3_300_658,
        companies: 294_282,
        sources: 227,
        ats_platforms: 93,
        telegram_channels: 95,
        computed_at: '2026-08-16T10:00:00Z',
        exact: true,
      }),
      '',
      {},
    );

    const scale = await client.catalogScale();

    // One call, not two list reads — that is what stops /about and /open showing
    // numbers measured at different moments.
    expect(seen.url).toBe('/api/v1/stats/catalog');
    expect(scale.open_jobs).toBe(3_300_658);
    expect(scale.companies).toBe(294_282);
    expect(scale.sources).toBe(227);
    expect(scale.telegram_channels).toBe(95);
    expect(scale.exact).toBe(true);
  });

  // Before the first worker run, and whenever Redis is unreachable, the backend answers
  // with an approximate job count and zeroes for the figures only the database holds.
  // The client must surface that rather than smoothing it over — a page showing
  // "0 companies" is worse than a page showing no companies stat at all.
  it('passes a degraded snapshot through unchanged', async () => {
    const client = createApi(
      snapshotFetch(
        {},
        { open_jobs: 3_150_000, companies: 0, sources: 227, telegram_channels: 0, exact: false },
      ),
      '',
      {},
    );

    const scale = await client.catalogScale();

    expect(scale.exact).toBe(false);
    expect(scale.open_jobs).toBe(3_150_000);
    expect(scale.companies).toBe(0);
    expect(scale.sources).toBe(227);
  });
});
