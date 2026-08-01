import { describe, it, expect } from 'vitest';
import { buildLocationPreferences, type LocationFields } from './profileLocation';

function fields(over: Partial<LocationFields> = {}): LocationFields {
  return {
    workModes: [],
    remoteRegions: [],
    remoteCountries: [],
    baseCountry: '',
    baseCity: '',
    relocOpen: false,
    relocRegions: [],
    relocCountries: [],
    relocCities: [],
    ...over,
  };
}

describe('buildLocationPreferences', () => {
  // The regression this module exists for. `base` is asked of every user now, so it can be
  // the ONLY thing they fill in — and the old emptiness proxy (no work modes → send null)
  // would throw it away, reproducing the exact data loss this change removes.
  it('keeps a base stated without any work format', () => {
    const loc = buildLocationPreferences(fields({ baseCountry: 'co', baseCity: 'Manizales' }));
    expect(loc).not.toBeNull();
    expect(loc?.base).toEqual({ country: 'co', city: 'Manizales' });
    expect(loc?.work_modes).toEqual([]);
  });

  it('keeps a base stated by a remote-only user', () => {
    const loc = buildLocationPreferences(fields({ workModes: ['remote'], baseCountry: 'co' }));
    expect(loc?.base.country).toBe('co');
  });

  it('still sends null when the user has stated nothing at all', () => {
    expect(buildLocationPreferences(fields())).toBeNull();
  });

  it('treats a whitespace-only base as nothing stated', () => {
    expect(buildLocationPreferences(fields({ baseCity: '   ' }))).toBeNull();
  });

  // The other gate is unchanged: a sub-form the current work format hides must not save
  // its lingering draft.
  it('drops a remote reach drafted before remote was deselected', () => {
    const loc = buildLocationPreferences(
      fields({ workModes: ['onsite'], remoteRegions: ['latam'], baseCountry: 'br' }),
    );
    expect(loc?.remote).toEqual({});
  });

  it('drops relocation targets when the user is not open to relocating', () => {
    const loc = buildLocationPreferences(
      fields({ workModes: ['onsite'], relocOpen: false, relocCities: ['Berlin'] }),
    );
    expect(loc?.relocation).toEqual({ open: false });
  });

  it('drops relocation targets for a user who takes no physical work', () => {
    const loc = buildLocationPreferences(
      fields({ workModes: ['remote'], relocOpen: true, relocCities: ['Berlin'] }),
    );
    expect(loc?.relocation).toEqual({ open: false });
  });

  it('carries the full block for a user who states everything', () => {
    const loc = buildLocationPreferences(
      fields({
        workModes: ['remote', 'onsite'],
        remoteRegions: ['latam'],
        baseCountry: 'br',
        baseCity: 'Florianópolis',
        relocOpen: true,
        relocCities: ['Berlin'],
      }),
    );
    expect(loc?.remote).toEqual({ regions: ['latam'], countries: [] });
    expect(loc?.base).toEqual({ country: 'br', city: 'Florianópolis' });
    expect(loc?.relocation).toEqual({ open: true, regions: [], countries: [], cities: ['Berlin'] });
  });
});
