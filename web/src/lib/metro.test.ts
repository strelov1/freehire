import { describe, expect, it } from 'vitest';
import { metroLabel } from './metro';

describe('metroLabel', () => {
  it('replaces the country suffix with "area"', () => {
    expect(metroLabel('Sacramento (US)')).toBe('Sacramento area');
    expect(metroLabel('Springfield MA (US)')).toBe('Springfield MA area');
    expect(metroLabel('San Francisco Bay Area (US)')).toBe('San Francisco Bay Area');
  });
  it('strips only the trailing country suffix', () => {
    expect(metroLabel('Halle (Saale) (DE)')).toBe('Halle (Saale) area');
  });
  it('leaves a value without a suffix readable', () => {
    expect(metroLabel('Moscow')).toBe('Moscow area');
  });
});
