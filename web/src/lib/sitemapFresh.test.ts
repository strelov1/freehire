import { describe, expect, it } from 'vitest';

import {
  FRESH_SITEMAP_CHUNKS,
  JOB_SITEMAP_CHUNK,
  freshJobSitemapOffsets,
  sitemapIndexLocs,
} from './sitemap';

const ORIGIN = 'https://example.test';

function locs(overrides: Partial<Parameters<typeof sitemapIndexLocs>[0]> = {}) {
  return sitemapIndexLocs({
    origin: ORIGIN,
    roleCategorySlugs: ['backend'],
    jobOffsets: [0, JOB_SITEMAP_CHUNK],
    companyOffsets: [0],
    ...overrides,
  });
}

describe('freshJobSitemapOffsets', () => {
  it('opens one chunk-sized page per fresh chunk, starting at 0', () => {
    expect(freshJobSitemapOffsets()).toEqual([0, 10000, 20000, 30000]);
  });

  // The offsets are the window's tiling, so they must be derived from the same two
  // constants the backend's bound is. A hard-coded list would keep passing if the
  // chunk size moved, and the last sub-sitemap would then ask for an offset the API
  // answers with an empty page — a quarter of the window silently gone.
  it('derives its tiling from the chunk size and the chunk count', () => {
    const offsets = freshJobSitemapOffsets();
    expect(offsets).toHaveLength(FRESH_SITEMAP_CHUNKS);
    offsets.forEach((offset, i) => expect(offset).toBe(i * JOB_SITEMAP_CHUNK));
  });
});

describe('sitemapIndexLocs', () => {
  // The ordering guarantee this whole change exists for. A crawler reads the index
  // top-down and spends a finite budget; the freshest file has to be in front of the
  // 70 arbitrary-order paged ones or it buys nothing.
  it('lists every fresh job sub-sitemap before the first paged one', () => {
    const all = locs();
    const freshPositions = all
      .map((loc, i) => (loc.includes('/sitemap-jobs-fresh.xml') ? i : -1))
      .filter((i) => i >= 0);
    const firstPaged = all.findIndex((loc) => loc.includes('/sitemap-jobs.xml'));

    expect(freshPositions).toHaveLength(FRESH_SITEMAP_CHUNKS);
    expect(firstPaged).toBeGreaterThan(-1);
    for (const position of freshPositions) {
      expect(position).toBeLessThan(firstPaged);
    }
  });

  it('addresses each fresh sub-sitemap by its own offset', () => {
    const all = locs();
    for (const offset of freshJobSitemapOffsets()) {
      expect(all).toContain(`${ORIGIN}/sitemap-jobs-fresh.xml?offset=${offset}`);
    }
  });

  // The paged chunks and the company chunks are what they were; this change adds a
  // file in front of them and takes nothing away.
  it('still lists the paged job and company sub-sitemaps', () => {
    const all = locs();
    expect(all).toContain(`${ORIGIN}/sitemap-jobs.xml?offset=0`);
    expect(all).toContain(`${ORIGIN}/sitemap-jobs.xml?offset=${JOB_SITEMAP_CHUNK}`);
    expect(all).toContain(`${ORIGIN}/sitemap-companies.xml?offset=0`);
    expect(all).toContain(`${ORIGIN}/sitemap-pages.xml`);
    expect(all).toContain(`${ORIGIN}/sitemap-roles.xml?category=backend`);
  });

  // Every entry must be an absolute URL on the given origin: a sitemap index naming a
  // relative path is rejected wholesale by the protocol, so one bad entry costs the
  // whole file, not one sub-sitemap.
  it('emits absolute URLs on the given origin', () => {
    for (const loc of locs()) {
      expect(loc.startsWith(`${ORIGIN}/`)).toBe(true);
    }
  });
});
