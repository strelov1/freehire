import { render, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { listBaseCvs, listCvs, goto } = vi.hoisted(() => ({ listBaseCvs: vi.fn(), listCvs: vi.fn(), goto: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/state', () => ({ page: { data: {}, params: { id: 'cv-1' }, url: new URL('http://localhost/my/cvs/cv-1') } }));
vi.mock('$app/paths', () => ({
  resolve: (route: string, params?: Record<string, string>) =>
    route.replace('[slug]', params?.slug ?? '').replace('[id]', params?.id ?? ''),
}));
vi.mock('#lib/api.js', () => ({ api: { listBaseCvs, listCvs } }));

import Page from './+page.svelte';

beforeEach(() => {
  for (const m of [listBaseCvs, listCvs, goto]) m.mockReset();
});

// A CV's address is one door to the one workspace, whichever kind of CV it is.
describe('/my/cvs/[id]', () => {
  it('opens a base CV in the workspace\'s base mode', async () => {
    listBaseCvs.mockResolvedValue([{ id: 'cv-1' }]);
    listCvs.mockResolvedValue([]);
    render(Page);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/base/cv-1', { replaceState: true }));
  });

  it('sends a tailored copy to its vacancy workspace, as this address always did', async () => {
    listBaseCvs.mockResolvedValue([{ id: 'other' }]);
    listCvs.mockResolvedValue([{ id: 'cv-1', job_slug: 'sre-acme-1a2b' }]);
    render(Page);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/sre-acme-1a2b?cv=cv-1', { replaceState: true }));
  });

  it('sends an unknown id back to the list', async () => {
    listBaseCvs.mockResolvedValue([]);
    listCvs.mockResolvedValue([]);
    render(Page);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/my/cvs', { replaceState: true }));
  });
});
