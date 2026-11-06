import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api.js';

const api = {
  getJob: vi.fn(),
  getJobCopies: vi.fn(),
  getApplyForm: vi.fn(),
  facetCounts: vi.fn(),
};

vi.mock('#lib/server/api.js', () => ({ serverApi: () => api }));

const { load } = await import('./+page.server.js');

type LoadEvent = Parameters<typeof load>[0];

// Just enough of a Job for the see-also builder: no facets, so no collection matches.
const job = { public_slug: 'a-1', title: 'Dev', enrichment: {}, skills: [], countries: [], regions: [], collections: [] };

function run(slug: string) {
  return load({ params: { slug }, fetch, url: new URL(`https://x/jobs/${slug}`) } as unknown as LoadEvent);
}

describe('job page load: application form', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getJobCopies.mockResolvedValue({ copies: [], total: 0 });
    api.facetCounts.mockResolvedValue(null);
  });

  it('does not ask for a form the detail response says was never captured', async () => {
    api.getJob.mockResolvedValue(job);

    const data = await run('a-1');

    expect(api.getApplyForm).not.toHaveBeenCalled();
    expect(data).toMatchObject({ applyForm: null });
  });

  it('fetches the form when one was captured, even with zero required questions', async () => {
    const form = { provider: 'greenhouse', questions: [] };
    api.getJob.mockResolvedValue({ ...job, apply_questions: 0 });
    api.getApplyForm.mockResolvedValue(form);

    const data = await run('a-1');

    expect(api.getApplyForm).toHaveBeenCalledWith('a-1');
    expect(data).toMatchObject({ applyForm: form });
  });

  it('degrades to no form when the form read fails', async () => {
    api.getJob.mockResolvedValue({ ...job, apply_questions: 2 });
    api.getApplyForm.mockRejectedValue(new Error('boom'));

    const data = await run('a-1');

    expect(data).toMatchObject({ applyForm: null });
  });

  it('does not ask for a form when the slug is dead', async () => {
    api.getJob.mockRejectedValue(new ApiError(404, 'not found'));

    await expect(run('gone')).rejects.toMatchObject({ status: 404 });
    expect(api.getApplyForm).not.toHaveBeenCalled();
  });
});
