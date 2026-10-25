import { describe, it, expect } from 'vitest';
import { avatarInitials, avatarColor, PALETTE } from './avatar';

describe('avatarInitials', () => {
  it('takes two initials from a multi-word name', () => {
    expect(avatarInitials('n8n Hiring Team', 'x@y.com')).toBe('NH');
    expect(avatarInitials('Milosh Zhivachki', 'x@y.com')).toBe('MZ');
  });

  it('takes one initial from a single word', () => {
    expect(avatarInitials('g-mate', 'x@y.com')).toBe('G');
  });

  it('falls back to the address when no name', () => {
    expect(avatarInitials('', 'no-reply@ashbyhq.com')).toBe('N');
  });

  it('falls back to ? when empty', () => {
    expect(avatarInitials('', '')).toBe('?');
    expect(avatarInitials('   ', '')).toBe('?');
  });

  it('is always uppercase', () => {
    expect(avatarInitials('acme corp', '')).toBe('AC');
  });
});

describe('avatarColor', () => {
  it('is deterministic for the same seed', () => {
    expect(avatarColor('a@b.com')).toBe(avatarColor('a@b.com'));
  });

  it('returns a palette hex', () => {
    expect(avatarColor('anything')).toMatch(/^#[0-9a-f]{6}$/);
  });

  it('spreads different seeds across the palette', () => {
    const seeds = ['a@b.com', 'c@d.com', 'e@f.com', 'g@h.com', 'i@j.com'];
    const colors = new Set(seeds.map(avatarColor));
    expect(colors.size).toBeGreaterThan(1);
  });
});

// WCAG 2.x relative luminance and contrast ratio, for the white initials on each colour.
function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

describe('avatar palette', () => {
  it.each(PALETTE)('%s holds white 12px initials at 4.5:1 or better', (hex) => {
    expect(1.05 / (luminance(hex) + 0.05)).toBeGreaterThanOrEqual(4.5);
  });

  it('has no repeated colour', () => {
    expect(new Set(PALETTE).size).toBe(PALETTE.length);
  });
});
