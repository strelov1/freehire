import { describe, expect, it } from 'vitest';
import { hasSessionCookie } from './authCookie';

describe('hasSessionCookie', () => {
  it('finds the session cookie among others', () => {
    expect(hasSessionCookie('_ga=GA1.1; hire_token=abc; theme=dark')).toBe(true);
    expect(hasSessionCookie('hire_token=abc')).toBe(true);
  });

  it('is false with no cookies at all', () => {
    expect(hasSessionCookie(null)).toBe(false);
    expect(hasSessionCookie('')).toBe(false);
  });

  // Analytics sets a cookie for nearly every first-time visitor, so "has cookies"
  // is not the question being asked.
  it('is false when other cookies are present but no session', () => {
    expect(hasSessionCookie('_ga=GA1.1; theme=dark')).toBe(false);
  });

  // A substring test would call this a session and mark the response private,
  // which would quietly disable caching for everyone carrying such a cookie.
  it('does not mistake a cookie whose name merely ends the same way', () => {
    expect(hasSessionCookie('not_hire_token=abc')).toBe(false);
    expect(hasSessionCookie('x=1; not_hire_token=abc')).toBe(false);
  });
});
