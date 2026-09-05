import { describe, it, expect } from 'vitest';
import { realityBadge } from './reality';
import type { Reality } from './generated/contracts';

const base = (over: Partial<Reality>): Reality => ({
  class: 'fresh',
  age_days: 2,
  repost_count: 1,
  mass_posting_count: 1,
  fake_freshness: false,
  ...over,
});


describe('realityBadge', () => {
  it('returns null for a fresh job (no badge)', () => {
    expect(realityBadge(base({ class: 'fresh' }))).toBeNull();
  });

  it('returns null when reality is absent', () => {
    expect(realityBadge(undefined)).toBeNull();
    expect(realityBadge(null)).toBeNull();
  });

  it('marks a likely-evergreen job with a warning tone and full tooltip', () => {
    const b = realityBadge(base({ class: 'likely-evergreen', age_days: 240, repost_count: 6 }));
    expect(b?.tone).toBe('warn');
    expect(b?.tooltip).toContain('240');
    expect(b?.tooltip).toContain('6'); // reposted 6×
  });

  it('reports fake-freshness in the tooltip when the posting date was refreshed', () => {
    const b = realityBadge(base({ class: 'likely-evergreen', age_days: 300, fake_freshness: true }));
    expect(b?.tooltip.toLowerCase()).toContain('refreshed');
  });

  it('marks a stale job with a muted tone stating its age in the chip label', () => {
    const b = realityBadge(base({ class: 'stale', age_days: 120 }));
    expect(b?.tone).toBe('muted');
    expect(b?.label).toBe('Open 120d');
    expect(b?.tooltip).toContain('120');
  });

  it('keeps the age out of the evidence string so it is not shown twice', () => {
    const b = realityBadge(base({ class: 'stale', age_days: 120, repost_count: 3 }));
    expect(b?.evidence).toBe('reposted 3×');
    expect(b?.evidence).not.toContain('120');
    expect(b?.evidence).not.toContain('Open');
  });

  it('leaves the evidence empty when a stale job has no evidence beyond its age', () => {
    const b = realityBadge(base({ class: 'stale', age_days: 30 }));
    expect(b?.evidence).toBe('');
  });
});
