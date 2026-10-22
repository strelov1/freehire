import { describe, expect, it, vi } from 'vitest';
import { ApiError, MovedError } from '#lib/api.js';

const getCompany = vi.fn();
vi.mock('./api', () => ({ serverApi: () => ({ getCompany }) }));

const { loadDiscussionSubject } = await import('./discussionSubject');

const acme = { slug: 'acme', name: 'Acme' };

describe('loadDiscussionSubject for a company', () => {
  it('hands the whole company through, so the page can draw its header and tabs', async () => {
    getCompany.mockResolvedValueOnce({ company: acme });

    const state = await loadDiscussionSubject(fetch, 'company', 'acme');

    expect(state.company).toEqual(acme);
    expect(state.subject).not.toBeNull();
  });

  it('hands through the canonical company for a merged slug, whose slug then differs', async () => {
    getCompany.mockRejectedValueOnce(new MovedError('acme')).mockResolvedValueOnce({ company: acme });

    const state = await loadDiscussionSubject(fetch, 'company', 'acme-inc');

    expect(state.company?.slug).toBe('acme');
  });

  it('carries no company when there is none', async () => {
    getCompany.mockRejectedValueOnce(new ApiError(404, 'not found'));

    const state = await loadDiscussionSubject(fetch, 'company', 'gone');

    expect(state.company).toBeUndefined();
    expect(state.absence).toBe('gone');
  });
});
