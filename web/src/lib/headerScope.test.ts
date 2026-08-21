import { describe, it, expect } from 'vitest';
import type { FacetStore, FacetSelection } from './facets';
import { summarizeScope, COMPANIES_SCOPE } from './headerScope';

// Minimal fake store: only `facet` is exercised by summarizeScope.
type Sel = { include?: string[]; exclude?: string[] };
function fakeStore(sel: Record<string, Sel>): Pick<FacetStore, 'facet'> {
  return {
    facet: (param: string): FacetSelection => ({
      include: sel[param]?.include ?? [],
      exclude: sel[param]?.exclude ?? [],
      matchAll: false,
    }),
  };
}

describe('summarizeScope', () => {
  it('no selection → neutral Location', () => {
    expect(summarizeScope(fakeStore({}))).toEqual({ icons: ['globe'], text: 'Location', extra: 0, label: 'Location' });
  });

  // The glyph already says "remote", so the word survives only in the accessible
  // name. With no geography picked the trigger keeps its neutral "Location" text —
  // a lone glyph would not read as something you can open.
  it('work format only → icon carries it, text stays the invitation', () => {
    expect(summarizeScope(fakeStore({ work_mode: { include: ['remote'] } }))).toEqual({
      icons: ['remote'],
      text: 'Location',
      extra: 0,
      label: 'Remote',
    });
  });

  it('single region → region label', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu'] } }))).toEqual({
      icons: ['globe'],
      text: 'Europe',
      extra: 0,
      label: 'Europe',
    });
  });

  it('multiple regions → first + N roll-up', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu', 'uk'] } }))).toEqual({
      icons: ['globe'],
      text: 'Europe',
      extra: 1,
      label: 'Europe +1',
    });
  });

  it('country with no region → country label', () => {
    expect(summarizeScope(fakeStore({ countries: { include: ['DE'] } }))).toEqual({
      icons: ['globe'],
      text: 'Germany',
      extra: 0,
      label: 'Germany',
    });
  });

  it('work format + multiple regions → format icon, geo text +N', () => {
    expect(
      summarizeScope(fakeStore({ work_mode: { include: ['remote'] }, regions: { include: ['eu', 'uk'] } })),
    ).toEqual({ icons: ['remote'], text: 'Europe', extra: 1, label: 'Remote · Europe +1' });
  });

  it('excluded geo counts toward the +N roll-up', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu'], exclude: ['uk'] } }))).toEqual({
      icons: ['globe'],
      text: 'Europe',
      extra: 1,
      label: 'Europe +1',
    });
  });

  describe('worldwide', () => {
    it('leads the geo roll-up → a globe lapping the format icon, no words', () => {
      expect(
        summarizeScope(
          fakeStore({
            work_mode: { include: ['remote'] },
            regions: { include: ['global', 'eu', 'uk'] },
            countries: { include: ['DE'] },
          }),
        ),
      ).toEqual({ icons: ['remote', 'globe'], text: '', extra: 3, label: 'Remote · Worldwide +3' });
    });

    it('alone → the globe alone', () => {
      expect(summarizeScope(fakeStore({ regions: { include: ['global'] } }))).toEqual({
        icons: ['globe'],
        text: '',
        extra: 0,
        label: 'Worldwide',
      });
    });

    it('behind another region keeps that region as the text', () => {
      expect(summarizeScope(fakeStore({ regions: { include: ['eu', 'global'] } }))).toEqual({
        icons: ['globe'],
        text: 'Europe',
        extra: 1,
        label: 'Europe +1',
      });
    });

    it('a city named global is a place, not everywhere', () => {
      expect(summarizeScope(fakeStore({ cities: { include: ['global'] } }))).toEqual({
        icons: ['globe'],
        text: 'global',
        extra: 0,
        label: 'global',
      });
    });
  });

  describe('company spec', () => {
    it('summarizes remote_regions', () => {
      expect(summarizeScope(fakeStore({ remote_regions: { include: ['eu'] } }), COMPANIES_SCOPE)).toEqual({
        icons: ['globe'],
        text: 'Europe',
        extra: 0,
        label: 'Europe',
      });
    });

    it('rolls up regions then remote_regions', () => {
      expect(
        summarizeScope(fakeStore({ regions: { include: ['eu'] }, remote_regions: { include: ['uk'] } }), COMPANIES_SCOPE),
      ).toEqual({ icons: ['globe'], text: 'Europe', extra: 1, label: 'Europe +1' });
    });

    it('ignores work_mode (companies have no work format)', () => {
      expect(summarizeScope(fakeStore({ work_mode: { include: ['remote'] } }), COMPANIES_SCOPE)).toEqual({
        icons: ['globe'],
        text: 'Location',
        extra: 0,
        label: 'Location',
      });
    });
  });
});
