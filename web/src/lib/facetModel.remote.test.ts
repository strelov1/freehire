import { describe, it, expect } from 'vitest';
import {
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  appliesWorkModeRemoteAnywhere,
  workModeRemoteLabel,
} from './facetModel';
import { savedSearchQuery } from './savedSearchQuery';

describe('workModeRemoteAnywhere', () => {
  // The server reads the param as a boolean, so anything but "true" is unset — the
  // serializer must write exactly that, and the parser must read it back.
  it('round-trips as work_mode_remote_anywhere=true and is absent when off', () => {
    expect(filtersToParams(emptyFilters()).has('work_mode_remote_anywhere')).toBe(false);
    const p = filtersToParams({ ...emptyFilters(), workModeRemoteAnywhere: true });
    expect(p.get('work_mode_remote_anywhere')).toBe('true');
    expect(filtersFromParams(p).workModeRemoteAnywhere).toBe(true);
    expect(filtersFromParams(new URLSearchParams('work_mode_remote_anywhere=false')).workModeRemoteAnywhere).toBe(
      false,
    );
    expect(filtersFromParams(new URLSearchParams('work_mode_remote_anywhere=%20TRUE')).workModeRemoteAnywhere).toBe(
      true,
    );
  });

  it('carries the remote reach only with the modifier, and labels it', () => {
    const on = {
      ...emptyFilters(),
      workModeRemoteAnywhere: true,
      workModeRemoteRegions: ['eu'],
      workModeRemoteCountries: ['us'],
    };
    const p = filtersToParams(on);
    expect(p.get('work_mode_remote_regions')).toBe('eu');
    expect(p.get('work_mode_remote_countries')).toBe('us');
    const back = filtersFromParams(p);
    expect([back.workModeRemoteRegions, back.workModeRemoteCountries]).toEqual([['eu'], ['us']]);
    expect(workModeRemoteLabel(back)).toMatch(/^Remote within Europe, /);
    expect(workModeRemoteLabel(emptyFilters())).toBe('Remote from anywhere');
    // Without the modifier the reach means nothing, so it is neither written nor read.
    expect(filtersToParams({ ...on, workModeRemoteAnywhere: false }).has('work_mode_remote_regions')).toBe(false);
    expect(filtersFromParams(new URLSearchParams('work_mode_remote_regions=eu')).workModeRemoteRegions).toEqual([]);
  });

  // A saved search keeps it: the digest matcher replays the stored query through the
  // same server builder, so the alert must mail the set the subscriber saw.
  it('reaches a saved search', () => {
    const q = savedSearchQuery({
      ...emptyFilters(),
      workModeRemoteAnywhere: true,
      facets: { ...emptyFilters().facets, work_mode: { include: ['remote', 'hybrid'], exclude: [], matchAll: false } },
    });
    expect(new URLSearchParams(q).get('work_mode_remote_anywhere')).toBe('true');
  });

  it('applies only for remote plus a physical mode plus a location facet', () => {
    const base = emptyFilters();
    const modes = (include: string[]) => ({
      ...base,
      facets: { ...base.facets, work_mode: { include, exclude: [], matchAll: false } },
    });
    const withCountry = (f: typeof base) => ({
      ...f,
      facets: { ...f.facets, countries: { include: ['de'], exclude: [], matchAll: false } },
    });

    expect(appliesWorkModeRemoteAnywhere(modes(['remote', 'hybrid']))).toBe(false); // no location
    expect(appliesWorkModeRemoteAnywhere(modes(['remote']))).toBe(false); // no physical mode
    expect(appliesWorkModeRemoteAnywhere(modes(['hybrid', 'onsite']))).toBe(false); // no remote
    expect(appliesWorkModeRemoteAnywhere(withCountry(modes(['remote'])))).toBe(false);
    expect(appliesWorkModeRemoteAnywhere(withCountry(modes(['hybrid', 'onsite'])))).toBe(false);
    expect(appliesWorkModeRemoteAnywhere(withCountry(modes(['remote', 'hybrid'])))).toBe(true);
    expect(appliesWorkModeRemoteAnywhere(withCountry(modes(['remote', 'onsite'])))).toBe(true);
    expect(appliesWorkModeRemoteAnywhere(withCountry(modes(['Remote', 'Hybrid'])))).toBe(true); // folded like the server
    const withRegion = (r: string) => ({
      ...modes(['remote', 'hybrid']),
      facets: { ...modes(['remote', 'hybrid']).facets, regions: { include: [r], exclude: [], matchAll: false } },
    });
    expect(appliesWorkModeRemoteAnywhere(withRegion('eu'))).toBe(true);
    expect(appliesWorkModeRemoteAnywhere(withRegion('not-a-region'))).toBe(false); // the server ignores it too
    const andMode = withCountry(modes(['remote', 'hybrid']));
    const andModeFilters = {
      ...andMode,
      facets: { ...andMode.facets, work_mode: { include: ['remote', 'hybrid'], exclude: [], matchAll: true } },
    };
    expect(appliesWorkModeRemoteAnywhere(andModeFilters)).toBe(false); // the server ignores it under _mode=and
  });

  // Inert (off, or on under a condition it cannot act on) it narrows nothing, so a
  // badge for it would lie — same rule the other modifiers follow. The base facets
  // (the mode and country includes) count for themselves; the modifier adds one only
  // in the live case.
  it('counts as an active filter only when it changes the answer', () => {
    const modes = (include: string[]) => (f: ReturnType<typeof emptyFilters>) => ({
      ...f,
      facets: {
        ...f.facets,
        work_mode: { include, exclude: [], matchAll: false },
        countries: { include: ['de'], exclude: [], matchAll: false },
      },
    });
    const inert = modes(['hybrid', 'onsite'])({ ...emptyFilters(), workModeRemoteAnywhere: true });
    const live = modes(['remote', 'hybrid'])({ ...emptyFilters(), workModeRemoteAnywhere: true });
    expect(activeFilterCount(live) - activeFilterCount({ ...live, workModeRemoteAnywhere: false })).toBe(1);
    expect(activeFilterCount(inert) - activeFilterCount({ ...inert, workModeRemoteAnywhere: false })).toBe(0);
  });
});
