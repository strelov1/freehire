import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { listBaseCvs, listCvs, getCv, updateCv, reseedCv, listCvFonts, getPhoto, goto } = vi.hoisted(() => ({
  listBaseCvs: vi.fn(),
  listCvs: vi.fn(),
  getCv: vi.fn(),
  updateCv: vi.fn(),
  reseedCv: vi.fn(),
  listCvFonts: vi.fn(),
  getPhoto: vi.fn(),
  goto: vi.fn(),
}));

vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/state', () => ({ page: { data: {}, params: { id: 'cv-1' }, url: new URL('http://localhost/my/cvs/cv-1') } }));
vi.mock('$app/paths', () => ({
  resolve: (route: string, params?: Record<string, string>) =>
    route.replace('[slug]', params?.slug ?? '').replace('[id]', params?.id ?? ''),
}));
vi.mock('#lib/api.js', () => ({
  api: { listBaseCvs, listCvs, getCv, updateCv, reseedCv, listCvFonts, getPhoto, cvPdfUrl: (id: string) => `/pdf/${id}` },
  ApiError: class extends Error {},
}));
// The preview measures layout, which jsdom cannot do; the editor's own behaviour is what is under test.
vi.mock('#lib/tailor/CvHtmlPreview.svelte', () => ({ default: () => {} }));

import Page from './+page.svelte';

const record = (over = {}) => ({
  id: 'cv-1',
  title: 'Backend',
  template_id: 'classic-ats',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-02T10:00:00Z',
  agent_session_id: '',
  tracer_links_enabled: false,
  document: { header: { full_name: 'Ada' }, summary: 'one' },
  ...over,
});

beforeEach(() => {
  for (const m of [listBaseCvs, listCvs, getCv, updateCv, reseedCv, listCvFonts, getPhoto, goto]) m.mockReset();
  listCvFonts.mockResolvedValue([]);
  getPhoto.mockResolvedValue({ present: false });
  updateCv.mockResolvedValue({});
});

describe('/my/cvs/[id]', () => {
  it('opens the editor for a base CV and autosaves a title change once, after a pause', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    listBaseCvs.mockResolvedValue([{ id: 'cv-1', title: 'Backend', is_default: true, hand_edited: false }]);
    getCv.mockResolvedValue(record());
    render(Page);

    const input = (await screen.findByLabelText('CV title')) as HTMLInputElement;
    expect(input.value).toBe('Backend');
    expect(goto).not.toHaveBeenCalled();

    await fireEvent.input(input, { target: { value: 'Backend v2' } });
    await fireEvent.input(input, { target: { value: 'Backend v3' } });
    expect(updateCv).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(900);
    await waitFor(() => expect(updateCv).toHaveBeenCalledTimes(1));
    expect(updateCv.mock.calls[0]?.[0]).toBe('cv-1');
    expect(updateCv.mock.calls[0]?.[1]).toMatchObject({ title: 'Backend v3', template_id: 'classic-ats' });
    vi.useRealTimers();
  });

  it('sends a tailored copy to its workspace, as this address always did', async () => {
    listBaseCvs.mockResolvedValue([{ id: 'other', title: 'Base', is_default: true, hand_edited: false }]);
    listCvs.mockResolvedValue([{ id: 'cv-1', job_slug: 'sre-acme-1a2b' }]);
    render(Page);
    await waitFor(() => expect(goto).toHaveBeenCalled());
    expect(goto).toHaveBeenCalledWith('/tailor/sre-acme-1a2b?cv=cv-1', { replaceState: true });
    expect(getCv).not.toHaveBeenCalled();
  });

  it('sends an unknown id back to the list', async () => {
    listBaseCvs.mockResolvedValue([]);
    listCvs.mockResolvedValue([]);
    render(Page);
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/my/cvs', { replaceState: true }));
  });

  it('asks before replacing hand edits with the current CV, then reseeds THIS base', async () => {
    listBaseCvs.mockResolvedValue([{ id: 'cv-1', title: 'Backend', is_default: true, hand_edited: true }]);
    getCv.mockResolvedValue(record());
    reseedCv.mockResolvedValue(record({ title: 'Backend', document: { header: {}, summary: 'from seed' } }));
    render(Page);

    await fireEvent.click(await screen.findByText('Update from my CV'));
    expect(reseedCv).not.toHaveBeenCalled();
    await screen.findByText(/will be replaced/);
    await fireEvent.click(screen.getByText('Replace content'));
    await waitFor(() => expect(reseedCv).toHaveBeenCalledWith('cv-1'));
  });

  it('waits for an autosave already in flight before reseeding, so a late PUT cannot undo the refresh', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    listBaseCvs.mockResolvedValue([{ id: 'cv-1', title: 'Backend', is_default: true, hand_edited: true }]);
    getCv.mockResolvedValue(record());
    let finishSave: () => void = () => {};
    updateCv.mockReturnValue(new Promise<void>((resolve) => (finishSave = resolve)));
    reseedCv.mockResolvedValue(record({ document: { header: {}, summary: 'from seed' } }));
    render(Page);

    const input = (await screen.findByLabelText('CV title')) as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'Edited' } });
    await vi.advanceTimersByTimeAsync(900); // the debounce fires; the PUT is now on the wire
    await waitFor(() => expect(updateCv).toHaveBeenCalledTimes(1));

    await fireEvent.click(screen.getByText('Update from my CV'));
    await fireEvent.click(await screen.findByText('Replace content'));
    await vi.advanceTimersByTimeAsync(50);
    expect(reseedCv).not.toHaveBeenCalled(); // still waiting on the save

    finishSave();
    await waitFor(() => expect(reseedCv).toHaveBeenCalledWith('cv-1'));
    vi.useRealTimers();
  });
});
