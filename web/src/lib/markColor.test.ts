import { describe, expect, test } from 'vitest';
import { glyphColorFor } from './markColor';

describe('glyphColorFor', () => {
  test('returns white for a dark brand background', () => {
    expect(glyphColorFor('#3776AB')).toBe('#ffffff');
  });

  test('returns near-black for a light brand background', () => {
    expect(glyphColorFor('#61DAFB')).toBe('#16181d');
  });
});
