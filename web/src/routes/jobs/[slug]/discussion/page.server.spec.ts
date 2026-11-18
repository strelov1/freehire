import { isRedirect } from '@sveltejs/kit';
import { beforeEach, expect, it, vi } from 'vitest';

const { getJob, loadDiscussionIndex } = vi.hoisted(() => ({
  getJob: vi.fn(),
  loadDiscussionIndex: vi.fn(async () => ({ slug: 'old-slug', threads: [] })),
}));
vi.mock('$app/paths', () => ({ resolve: (_route: string, params: { slug: string }) => `/jobs/${params.slug}` }));
vi.mock('#lib/server/api.js', () => ({ serverApi: () => ({ getJob }) }));
vi.mock('#lib/server/community.js', () => ({ loadDiscussionIndex }));

const { load } = await import('./+page.server');
const run = (slug: string) => load({ params: { slug }, fetch } as unknown as Parameters<typeof load>[0]);

beforeEach(() => vi.clearAllMocks());

it('sends the discussion page to the job page with the Discussion tab open', async () => {
  getJob.mockResolvedValue({ public_slug: 'acme-job' });
  const outcome = await Promise.resolve(run('acme-job')).catch((e: unknown) => e);
  if (!isRedirect(outcome)) throw new Error('expected a redirect');
  expect(outcome.status).toBe(301);
  expect(outcome.location).toBe('/jobs/acme-job?tab=discussion');
});

it.each([
  ['renamed', { public_slug: 'new-slug' }],
  ['gone', null],
])('keeps the discussion page for a %s job, whose topics live under the old slug', async (_case, job) => {
  if (job) getJob.mockResolvedValue(job);
  else getJob.mockRejectedValue(new Error('404'));
  await expect(run('old-slug')).resolves.toEqual({ slug: 'old-slug', threads: [] });
  expect(loadDiscussionIndex).toHaveBeenCalledWith(fetch, 'job', 'old-slug');
});
