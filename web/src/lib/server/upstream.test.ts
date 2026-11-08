import { isHttpError } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';

import { ApiError, createApi } from '#lib/api.js';
import { rethrowUpstream } from './upstream';

function thrownByUpstream(value: unknown): unknown {
  try {
    rethrowUpstream(value);
  } catch (error) {
    return error;
  }
}

describe('rethrowUpstream', () => {
  it('renders a timed-out SSR API request as 504 without exposing its internal URL', async () => {
    const fetch: typeof globalThis.fetch = (_input, init) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(init.signal?.reason), { once: true });
      });
    const api = createApi(fetch, 'http://internal-api', {}, 10);
    const result = await api.getJob('example-job').catch(thrownByUpstream);

    expect(isHttpError(result, 504)).toBe(true);
    expect(result).toMatchObject({ status: 504, body: { message: 'Gateway timeout' } });
  });

  it.each([
    [429, 'Too many requests'],
    [503, 'Service unavailable'],
    [504, 'Gateway timeout'],
  ])('preserves upstream status %i with a public message', (status, message) => {
    const result = thrownByUpstream(new ApiError(status, 'Private upstream details'));

    expect(isHttpError(result, status)).toBe(true);
    expect(result).toMatchObject({ status, body: { message } });
  });

  it('rethrows unexpected failures for normal error reporting', () => {
    const failure = new Error('Unexpected failure');
    expect(thrownByUpstream(failure)).toBe(failure);
  });

  it.each([404, 500])('leaves API status %i to the caller', (status) => {
    const failure = new ApiError(status, 'Original API failure');
    expect(thrownByUpstream(failure)).toBe(failure);
  });
});
