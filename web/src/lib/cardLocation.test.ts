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
    expect(cardLocation('New York, USA', ['us'])).toBe('New York');
    expect(cardLocation('London, UK', ['gb'])).toBe('London');
  });

  it('drops the arrangement words the work-mode chip already states', () => {
    expect(cardLocation('Remote', [])).toBeNull();
    expect(cardLocation('Remote, Poland', ['pl'])).toBeNull();
    expect(cardLocation('Hybrid; Warsaw', ['pl'])).toBe('Warsaw');
  });

  it('reads a trailing parenthetical the same way, keeping a real place in it', () => {
    expect(cardLocation('Remote (US)', ['us'])).toBeNull();
    expect(cardLocation('Remote (Berlin)', ['de'])).toBe('Berlin');
    expect(cardLocation('Lisbon (Hybrid)', ['pt'])).toBe('Lisbon');
  });

  it('adds nothing for an empty line', () => {
    expect(cardLocation('', ['hr'])).toBeNull();
    expect(cardLocation(undefined, undefined)).toBeNull();
  });
});
