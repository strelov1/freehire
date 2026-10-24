import { describe, expect, it } from 'vitest';
import { COUNTRY_OPTIONS } from './facets';
import { COUNTRY_CENTROIDS, LAND_PATH, projectToMap } from './worldMap';

describe('world map geometry', () => {
  // The facet vocabulary is the list of codes a search can return; a code missing here
  // would silently drop off the map.
  it('has a point for every country the countries facet can emit', () => {
    const missing = COUNTRY_OPTIONS.map((o) => o.value).filter((code) => !COUNTRY_CENTROIDS[code]);
    expect(missing).toEqual([]);
  });

  it('keeps every point inside the globe', () => {
    for (const [lat, lng] of Object.values(COUNTRY_CENTROIDS)) {
      expect(Math.abs(lat)).toBeLessThanOrEqual(90);
      expect(Math.abs(lng)).toBeLessThanOrEqual(180);
    }
  });

  it('is one closed SVG path', () => {
    expect(LAND_PATH.startsWith('M')).toBe(true);
    expect(LAND_PATH.endsWith('Z')).toBe(true);
  });
});

describe('projectToMap', () => {
  it('places a point in the 360x180 equirectangular box the land path uses', () => {
    expect(projectToMap(0, 0)).toEqual({ x: 180, y: 90 });
    expect(projectToMap(-10, -52)).toEqual({ x: 128, y: 100 });
  });
});
