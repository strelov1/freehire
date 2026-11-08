import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api.js';

const api = { getJob: vi.fn(), getCompany: vi.fn() };
vi.mock('#lib/server/api.js', () => ({ serverApi: () => api }));

const { GET } = await import('./+server.js');
type GetEvent = Parameters<typeof GET>[0];

function run() {
  return GET({ params: { slug: 'acme-job' }, fetch } as unknown as GetEvent);
}

describe('job referral people redirect', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getJob.mockResolvedValue({ public_slug: 'acme-job', company: 'Acme', company_slug: 'acme' });
  });

  it('opens the stored company people tab', async () => {
    api.getCompany.mockResolvedValue({
      company: { company_info: { linkedin: 'https://www.linkedin.com/company/acme/?trk=site' } },
    });

    await expect(run()).rejects.toMatchObject({
      status: 302,
      location: 'https://www.linkedin.com/company/acme/people/',
    });
    expect(api.getCompany).toHaveBeenCalledWith('acme', 1, 0);
  });

  it('searches by company name when the company lookup fails', async () => {
    api.getCompany.mockRejectedValue(new Error('unavailable'));

    await expect(run()).rejects.toMatchObject({
      status: 302,
      location: 'https://www.linkedin.com/search/results/people/?keywords=Acme',
    });
  });

  it('returns 404 for an unknown job without looking up a company', async () => {
    api.getJob.mockRejectedValue(new ApiError(404, 'not found'));

    await expect(run()).rejects.toMatchObject({ status: 404 });
    expect(api.getCompany).not.toHaveBeenCalled();
  });
});
