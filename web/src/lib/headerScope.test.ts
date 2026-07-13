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
    expect(summarizeScope(fakeStore({}))).toEqual({ icon: 'globe', label: 'Location' });
  });

  it('work format only → format icon + label', () => {
    expect(summarizeScope(fakeStore({ work_mode: { include: ['remote'] } }))).toEqual({
      icon: 'remote',
      label: 'Remote',
    });
  });

  it('single region → region label', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu'] } }))).toEqual({
      icon: 'globe',
      label: 'Europe',
    });
  });

  it('multiple regions → first + N roll-up', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu', 'uk'] } }))).toEqual({
      icon: 'globe',
      label: 'Europe +1',
    });
  });

  it('country with no region → country label', () => {
    expect(summarizeScope(fakeStore({ countries: { include: ['DE'] } }))).toEqual({
      icon: 'globe',
      label: 'Germany',
    });
  });

  it('work format + multiple regions → format · geo +N', () => {
    expect(
      summarizeScope(fakeStore({ work_mode: { include: ['remote'] }, regions: { include: ['eu', 'uk'] } })),
    ).toEqual({ icon: 'remote', label: 'Remote · Europe +1' });
  });

  it('excluded geo counts toward the +N roll-up', () => {
    expect(summarizeScope(fakeStore({ regions: { include: ['eu'], exclude: ['uk'] } }))).toEqual({
      icon: 'globe',
      label: 'Europe +1',
    });
  });

  describe('company spec', () => {
    it('summarizes remote_regions', () => {
      expect(summarizeScope(fakeStore({ remote_regions: { include: ['eu'] } }), COMPANIES_SCOPE)).toEqual({
        icon: 'globe',
        label: 'Europe',
      });
    });

    it('rolls up regions then remote_regions', () => {
      expect(
        summarizeScope(fakeStore({ regions: { include: ['eu'] }, remote_regions: { include: ['uk'] } }), COMPANIES_SCOPE),
      ).toEqual({ icon: 'globe', label: 'Europe +1' });
    });

    it('ignores work_mode (companies have no work format)', () => {
      expect(summarizeScope(fakeStore({ work_mode: { include: ['remote'] } }), COMPANIES_SCOPE)).toEqual({
        icon: 'globe',
        label: 'Location',
      });
    });
  });
});
