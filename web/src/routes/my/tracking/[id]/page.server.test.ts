import { beforeEach, expect, it, vi } from 'vitest';
import { isRedirect } from '@sveltejs/kit';
import type { MyJob } from '#lib/types.js';
import { load } from './+page.server';

const { loadBoard, getTrackedApplication } = vi.hoisted(() => ({ loadBoard: vi.fn(), getTrackedApplication: vi.fn() }));
vi.mock('#lib/signin.js', () => ({ signinUrl: () => '/login' }));
vi.mock('#lib/server/tracking.js', () => ({ loadBoard }));
vi.mock('#lib/server/api.js', () => ({ serverApi: () => ({ getTrackedApplication }) }));
vi.mock('#lib/api.js', () => ({
  ApiError: class ApiError extends Error {
    status = 404;
  },
}));
import { ApiError } from '#lib/api.js';

const event = {
  parent: async () => ({ user: { id: 1 } }),
  params: { id: 'older-job' },
  url: new URL('http://localhost/my/tracking/older-job?tab=auto_apply'),
  request: new Request('http://localhost', { headers: { cookie: 'hire_token=test' } }),
  fetch,
};

beforeEach(() => {
  loadBoard.mockReset();
  getTrackedApplication.mockReset();
});

it('loads an auto-apply application outside the first board page', async () => {
  loadBoard.mockResolvedValue([]);
  getTrackedApplication.mockResolvedValue({
    job: { public_slug: 'older-job', title: 'Engineer', company_slug: 'acme' },
    stage: 'preparing',
    emails: [],
    auto_apply: { status: 'blocked' },
  });
  const result = await load(event as never);
  expect(result?.board?.find((row: MyJob) => row.id === 'older-job')?.auto_apply_status).toBe('blocked');
});

it('falls back to the vacancy if its tracking record was removed', async () => {
  loadBoard.mockResolvedValue([]);
  getTrackedApplication.mockResolvedValue({
    job: { public_slug: 'older-job', title: 'Engineer' },
    stage: null,
    applied_at: null,
    saved_at: null,
    emails: [],
  });
  try {
    await load(event as never);
    throw new Error('expected redirect');
  } catch (e) {
    expect(isRedirect(e) && e.location).toBe('/jobs/older-job');
  }
});

it('falls back to the vacancy if the application read returns 404', async () => {
  loadBoard.mockResolvedValue([]);
  getTrackedApplication.mockRejectedValue(new ApiError(404, 'Not found'));
  await expect(load(event as never)).rejects.toMatchObject({ status: 302, location: '/jobs/older-job' });
});

it('keeps a failed board read retryable when the target application can be loaded', async () => {
  loadBoard.mockResolvedValue(undefined);
  getTrackedApplication.mockResolvedValue({
    job: { public_slug: 'older-job', title: 'Engineer' },
    stage: 'preparing',
    emails: [],
  });
  const result = await load(event as never);
  expect(result?.board).toBeUndefined();
  expect(result?.initialItem?.id).toBe('older-job');
});
