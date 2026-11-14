import { describe, expect, it } from 'vitest';
import { ApiError } from './api-core';
import { searchRateLimit } from './searchRateLimit';

const refusal = (tier: string, upgrade: boolean) =>
  new ApiError(429, 'too many requests', {
    code: 'rate_limited',
    budget: 'publicread',
    tier,
    limit: 300,
    retry_after_seconds: 42,
    upgrade_available: upgrade,
  });

describe('searchRateLimit', () => {
  it('offers a higher plan for a Free search refusal and states the wait', () => {
    expect(searchRateLimit(refusal('free', true))).toEqual({
      title: 'Search limit reached',
      description:
        'Your Free search limit is 300 requests per minute. Try again in 42 seconds, or upgrade for a higher search limit.',
      upgrade: true,
    });
  });
  it('offers guests the same upgrade path', () => {
    expect(searchRateLimit(refusal('anonymous', true))?.description).toContain('guest search limit');
    expect(searchRateLimit(refusal('anonymous', true))?.upgrade).toBe(true);
  });
  it('does not promise that upgrading resolves a paid or infrastructure limit', () => {
    expect(searchRateLimit(refusal('pro', false))?.upgrade).toBe(false);
    expect(searchRateLimit(new ApiError(429, 'nginx refusal'))).toEqual({
      title: 'Too many search requests',
      description: 'Please wait a moment before searching again.',
      upgrade: false,
    });
  });
  it('keeps ordinary faults in the existing error state', () => {
    expect(searchRateLimit(new ApiError(500, 'failed'))).toBeNull();
    expect(searchRateLimit(new TypeError('Failed to fetch'))).toBeNull();
  });
  it('does not invent tariff details for malformed refusal metadata', () => {
    expect(
      searchRateLimit(
        new ApiError(429, 'refused', {
          code: 'rate_limited',
          budget: 'publicread',
          tier: 'free',
          limit: -1,
          retry_after_seconds: 'soon',
          upgrade_available: true,
        }),
      )?.upgrade,
    ).toBe(false);
  });
});
