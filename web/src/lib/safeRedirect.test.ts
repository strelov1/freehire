import { describe, it, expect } from 'vitest';
import { safeRedirect } from './safeRedirect';

describe('safeRedirect', () => {
  it('accepts a plain rooted path', () => {
    expect(safeRedirect('/my/tracking')).toBe('/my/tracking');
  });

  it('preserves query and hash on an accepted path', () => {
    expect(safeRedirect('/jobs?q=go#top')).toBe('/jobs?q=go#top');
  });

  it('rejects null / empty / relative input', () => {
    expect(safeRedirect(null)).toBeNull();
    expect(safeRedirect('')).toBeNull();
    expect(safeRedirect('jobs')).toBeNull();
  });

  it('rejects a scheme-relative //host', () => {
    expect(safeRedirect('//evil.com')).toBeNull();
  });

  it('rejects an absolute off-origin URL', () => {
    expect(safeRedirect('https://evil.com/phish')).toBeNull();
  });

  // The bypass the old startsWith('//') guard missed: a backslash is normalized to
  // a slash by the URL parser, so /\evil.com resolves to //evil.com → off-origin.
  it('rejects a backslash scheme-relative bypass', () => {
    expect(safeRedirect('/\\evil.com')).toBeNull();
    expect(safeRedirect('/\\/evil.com')).toBeNull();
  });

  it('rejects a tab/control-character bypass', () => {
    expect(safeRedirect('/\t/evil.com')).toBeNull();
  });
});
