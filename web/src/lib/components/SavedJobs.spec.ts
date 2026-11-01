import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { JobCard, MyJob } from '#lib/types.js';
import { isSaved } from '#lib/savedJobs.svelte.js';
import { resetUserStores } from '#lib/userResource.svelte.js';
import SavedJobs from './SavedJobs.svelte';

vi.mock('$app/env', () => ({ browser: true }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/my/activity') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => true,
  currentUser: () => ({ id: 7 }),
}));
const { listMyJobs, listSavedSlugs, unsaveJob } = vi.hoisted(() => ({
  listMyJobs: vi.fn(), listSavedSlugs: vi.fn(), unsaveJob: vi.fn(),
}));
vi.mock('#lib/api.js', async (original) => {
  const actual = await original<typeof import('#lib/api.js')>();
  return { ...actual, api: {
    ...actual.api, listMyJobs, listSavedSlugs, unsaveJob,
    getProfile: vi.fn().mockResolvedValue(null),
  } };
});

function item(id: number): MyJob {
  const job = {
    public_slug: 'job-' + id, title: 'Role ' + id, company: 'Company ' + id,
    source: 'greenhouse', location: '', skills: [], countries: [], collections: [],
  } as JobCard;
  return {
    id: String(id), job, company_slug: 'company-' + id, role_title: 'Role ' + id,
    viewed_at: '', saved_at: '2026-11-01T00:00:00Z', applied_at: null, stage: null, notes: null,
    email_count: 0, last_activity_at: null, days_silent: null, silence_state: null,
    followed_up_at: null, cv_opened_at: null,
  };
}
const first = item(1);
const second = item(2);

beforeEach(() => {
  resetUserStores();
  listMyJobs.mockReset();
  listSavedSlugs.mockReset().mockResolvedValue(['job-1', 'job-2']);
  unsaveJob.mockReset().mockResolvedValue(undefined);
});

describe('SavedJobs removal', () => {
  it('loads remaining jobs after removing the last loaded card', async () => {
    listMyJobs.mockResolvedValueOnce({ items: [first], total: 2, hasMore: true })
      .mockResolvedValueOnce({ items: [second], total: 1, hasMore: false });
    render(SavedJobs);
    await fireEvent.click(await screen.findByRole('button', { name: 'Remove from saved' }));

    expect(await screen.findByText('Role 2')).toBeTruthy();
    expect(screen.queryByText('Role 1')).toBeNull();
    expect(listMyJobs).toHaveBeenLastCalledWith('saved', 20, 0);
  });

  it('finishes pagination before deleting so its offset cannot become stale', async () => {
    const next = Promise.withResolvers<{ items: MyJob[]; total: number; hasMore: boolean }>();
    listMyJobs.mockResolvedValueOnce({ items: [first], total: 2, hasMore: true })
      .mockReturnValueOnce(next.promise);
    render(SavedJobs);
    const remove = await screen.findByRole('button', { name: 'Remove from saved' });
    await fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    await waitFor(() => expect(listMyJobs).toHaveBeenCalledTimes(2));
    await fireEvent.click(remove);
    expect(unsaveJob).not.toHaveBeenCalled();

    next.resolve({ items: [second], total: 2, hasMore: false });
    await waitFor(() => expect(unsaveJob).toHaveBeenCalledWith('job-1'));
    await waitFor(() => expect(screen.queryByText('Role 1')).toBeNull());
    expect(screen.getByText('Role 2')).toBeTruthy();
    expect(listMyJobs).toHaveBeenLastCalledWith('saved', 20, 1);
  });

  it('does not seed the shared saved cache from a response after unmount', async () => {
    const response = Promise.withResolvers<{ items: MyJob[]; hasMore: boolean }>();
    listMyJobs.mockReturnValue(response.promise);
    const view = render(SavedJobs);
    await waitFor(() => expect(listMyJobs).toHaveBeenCalledOnce());
    view.unmount();
    resetUserStores();
    response.resolve({ items: [first], hasMore: false });
    await response.promise;
    expect(isSaved('job-1')).toBe(false);
  });
});
