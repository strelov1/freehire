import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';
import JobSearchError from './JobSearchError.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

describe('JobSearchError', () => {
  it('offers an upgrade and retry without replacing the current search', async () => {
    const retry = vi.fn();
    render(JobSearchError, {
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
    render(JobSearchError, { error: new ApiError(429, 'Too Many Requests'), onRetry: vi.fn() });
    expect(screen.getByText('Too many search requests')).toBeTruthy();
    expect(screen.queryByRole('link')).toBeNull();
  });
});

it('preserves the ordinary failure message for a failed jobs request', () => {
  render(JobSearchError, { error: new ApiError(500, 'fault'), onRetry: vi.fn() });
  expect(screen.getByText('Failed to load jobs.')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy();
});

it('keeps non-quota facet failures silent', () => {
  render(JobSearchError, { error: new ApiError(500, 'fault'), onlyRateLimit: true, onRetry: vi.fn() });
  expect(screen.queryByRole('alert')).toBeNull();
  expect(screen.queryByRole('status')).toBeNull();
});
