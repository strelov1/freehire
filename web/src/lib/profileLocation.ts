import type { LocationPreferences } from './types';

/** The flat form fields the profile editor holds while the user edits the location block. */
export interface LocationFields {
  workModes: string[];
  remoteRegions: string[];
  remoteCountries: string[];
  baseCountry: string;
  baseCity: string;
  relocOpen: boolean;
  relocRegions: string[];
  relocCountries: string[];
  relocCities: string[];
}

/**
 * Reassemble the editor's flat fields into the saved location block, or null when the user
 * has stated nothing at all.
 *
 * Two gates, and the difference between them is the whole point:
 *
 * - The "where would you take work" sub-forms (remote reach, relocation targets) ARE gated
 *   on the accepted work formats. A hidden sub-form's lingering draft is not saved, so
 *   toggling a format off and saving does not silently keep its targets.
 * - `base` is NOT gated. Where someone lives is a fact about them, not a preference
 *   conditional on the arrangements they accept, and it matters most for a remote worker:
 *   it governs their right to work, their taxation, and their overlap with a team.
 *
 * The emptiness check has to respect that second point. It used to be `workModes.length
 * === 0`, which was a correct proxy for "the user stated nothing" only while every
 * geographic field was hidden behind a work format. With `base` asked unconditionally, that
 * proxy silently discards the base of anyone who fills it in without picking a format —
 * the same data loss this change exists to remove, just through a different door.
 */
export function buildLocationPreferences(f: LocationFields): LocationPreferences | null {
  const wantsRemote = f.workModes.includes('remote');
  const wantsPhysical = f.workModes.includes('onsite') || f.workModes.includes('hybrid');
  const baseCountry = f.baseCountry.trim();
  const baseCity = f.baseCity.trim();

  if (f.workModes.length === 0 && !baseCountry && !baseCity) return null;

  return {
    work_modes: f.workModes,
    remote: wantsRemote ? { regions: f.remoteRegions, countries: f.remoteCountries } : {},
    base: { country: baseCountry || undefined, city: baseCity || undefined },
    relocation:
      wantsPhysical && f.relocOpen
        ? { open: true, regions: f.relocRegions, countries: f.relocCountries, cities: f.relocCities }
        : { open: false },
  };
}
