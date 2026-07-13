import { describe, it, expect } from 'vitest';
import { avatarInitials, avatarColor } from './avatar';

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
