import { describe, expect, it } from 'vitest';
import { COUNTRY_OPTIONS } from './facets';
import {
  COUNTRY_CENTROIDS,
  LAND_PATH,
  WORLD_VIEW,
  bubbleRadius,
  fitView,
  placeCities,
  projectToMap,
} from './worldMap';

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

describe('bubbleRadius', () => {
  it('tracks the square root of the count, so area follows the number', () => {
    const big = bubbleRadius(100, 100, 360);
    const quarter = bubbleRadius(25, 100, 360);
    expect(big - 2).toBeCloseTo(2 * (quarter - 2));
  });

  it('never shrinks a place with jobs to nothing', () => {
    expect(bubbleRadius(1, 1_000_000, 360)).toBeGreaterThan(2);
  });

  it('shrinks with the view, so a zoomed-in map does not draw country-sized bubbles', () => {
    expect(bubbleRadius(10, 10, 60)).toBeCloseTo(bubbleRadius(10, 10, 360) / 6);
  });

  it('does not grow past full size on the whole-world view', () => {
    expect(bubbleRadius(10, 10, 720)).toBe(bubbleRadius(10, 10, 360));
  });
});

describe('fitView', () => {
  it('shows the inhabited world when there is nothing to fit', () => {
    expect(fitView([])).toEqual(WORLD_VIEW);
  });

  it('zooms to a narrow search, keeping the 2:1 aspect', () => {
    const brazil = projectToMap(-10, -52);
    const v = fitView([brazil]);
    expect(v.w).toBe(60);
    expect(v.h).toBe(30);
    expect(v.x).toBeLessThan(brazil.x);
    expect(v.x + v.w).toBeGreaterThan(brazil.x);
    expect(v.y).toBeLessThan(brazil.y);
    expect(v.y + v.h).toBeGreaterThan(brazil.y);
  });

  it('never zooms past a 60 degree window', () => {
    const v = fitView([projectToMap(-23.5, -46.6), projectToMap(-22.9, -47.1)]);
    expect(v.w).toBe(60);
  });

  it('fits every point with room to spare', () => {
    const pts = [projectToMap(40, -100), projectToMap(52, 13)];
    const v = fitView(pts);
    for (const p of pts) {
      expect(p.x).toBeGreaterThan(v.x);
      expect(p.x).toBeLessThan(v.x + v.w);
      expect(p.y).toBeGreaterThan(v.y);
      expect(p.y).toBeLessThan(v.y + v.h);
    }
    expect(v.w).toBe(2 * v.h);
  });

  it('shows the whole world when the places span most of it', () => {
    expect(fitView([projectToMap(40, -100), projectToMap(35, 139)])).toEqual(WORLD_VIEW);
  });

  it('stays inside the inhabited band at its edges', () => {
    const v = fitView([projectToMap(64, -150)]);
    expect(v.x).toBeGreaterThanOrEqual(WORLD_VIEW.x);
    expect(v.y).toBeGreaterThanOrEqual(WORLD_VIEW.y);
    expect(v.y + v.h).toBeLessThanOrEqual(WORLD_VIEW.y + WORLD_VIEW.h);
  });
});

describe('placeCities', () => {
  const table = {
    br: { 'sao paulo': [-23.56, -46.63], campinas: [-22.9, -47.1] },
    mx: { 'mexico city': [19.43, -99.13] },
  } satisfies Record<string, Record<string, [number, number]>>;

  it('places the cities it can find in that country and omits the rest', () => {
    const placed = placeCities('br', [['São Paulo', 50], ['Campinas', 9], ['Mexico City', 4], ['Brazil', 3]], table);
    expect(placed).toEqual([
      { name: 'São Paulo', count: 50, lat: -23.56, lng: -46.63 },
      { name: 'Campinas', count: 9, lat: -22.9, lng: -47.1 },
    ]);
  });

  it('matches regardless of case, accents and stray whitespace', () => {
    const placed = placeCities('br', [['  SÃO   PAULO ', 1], ['campínas', 2]], table);
    expect(placed.map((c) => c.name)).toEqual(['  SÃO   PAULO ', 'campínas']);
  });

  it('keeps one bubble per place when two spellings name the same city', () => {
    const placed = placeCities('br', [['São Paulo', 50], ['Sao Paulo', 7]], table);
    expect(placed).toEqual([{ name: 'São Paulo', count: 50, lat: -23.56, lng: -46.63 }]);
  });

  it('places nothing for a country the table does not know', () => {
    expect(placeCities('zz', [['São Paulo', 1]], table)).toEqual([]);
  });
});

