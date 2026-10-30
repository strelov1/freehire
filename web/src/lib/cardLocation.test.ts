import { describe, expect, it } from 'vitest';
import { cardLocation } from './cardLocation';

describe('cardLocation', () => {
  it('keeps the city the flag does not name', () => {
    expect(cardLocation('Zagreb', ['hr'])).toBe('Zagreb');
    expect(cardLocation('Zagreb (Sesvete)', ['hr'])).toBe('Zagreb (Sesvete)');
    expect(cardLocation('Sveta Helena, Sveti Ivan Zelina', ['hr'])).toBe('Sveta Helena, Sveti Ivan Zelina');
  });

  it('drops the country the flag already names, by name or code', () => {
    expect(cardLocation('Berlin, Germany', ['de'])).toBe('Berlin');
    expect(cardLocation('Austin, TX, US', ['us'])).toBe('Austin, TX');
    expect(cardLocation('Germany', ['de'])).toBeNull();
    expect(cardLocation('JP - Sagamihara', ['jp'])).toBe('Sagamihara');
  });

  it('drops the arrangement words the work-mode chip already states', () => {
    expect(cardLocation('Remote', [])).toBeNull();
    expect(cardLocation('Remote, Poland', ['pl'])).toBeNull();
    expect(cardLocation('Hybrid; Warsaw', ['pl'])).toBe('Warsaw');
  });

  it('adds nothing for an empty line', () => {
    expect(cardLocation('', ['hr'])).toBeNull();
    expect(cardLocation(undefined, undefined)).toBeNull();
  });
});
