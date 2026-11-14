import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';
import SearchRateLimitNotice from './SearchRateLimitNotice.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

describe('SearchRateLimitNotice', () => {
  it('offers an upgrade and retry without replacing the current search', async () => {
    const retry = vi.fn();
    render(SearchRateLimitNotice, {
      error: new ApiError(429, 'refused', {
        code: 'rate_limited',
        budget: 'publicread',
        tier: 'free',
        limit: 300,
        retry_after_seconds: 30,
        upgrade_available: true,
      }),
      onRetry: retry,
    });
    expect(screen.getByText('Search limit reached')).toBeTruthy();
    expect(screen.getByText(/Try again in 30 seconds/)).toBeTruthy();
    const upgrade = screen.getByRole('link', { name: 'Upgrade for higher search limits' });
    expect(upgrade.getAttribute('href')).toBe('/pricing');
    expect(upgrade.getAttribute('target')).toBe('_blank');
    await fireEvent.click(screen.getByRole('button', { name: 'Retry search' }));
    expect(retry).toHaveBeenCalledOnce();
  });

  it('does not offer a tariff upgrade for an unclassified nginx refusal', () => {
    render(SearchRateLimitNotice, { error: new ApiError(429, 'Too Many Requests'), onRetry: vi.fn() });
    expect(screen.getByText('Too many search requests')).toBeTruthy();
    expect(screen.queryByRole('link')).toBeNull();
  });
});
