import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CvBaseItem } from '#lib/cv.js';

const { listCvs, listBaseCvs, listCvTemplates, duplicateCv, deleteCv, reseedBaseCv, goto, openCvIntake } = vi.hoisted(
  () => ({
    listCvs: vi.fn(),
    listBaseCvs: vi.fn(),
    listCvTemplates: vi.fn(),
    duplicateCv: vi.fn(),
    deleteCv: vi.fn(),
    reseedBaseCv: vi.fn(),
    goto: vi.fn(),
    openCvIntake: vi.fn(),
  }),
);

vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/cvs') } }));
vi.mock('$app/paths', () => ({
  resolve: (route: string, params: Record<string, string>) => route.replace('[id]', params.id ?? ''),
}));
vi.mock('#lib/cvIntakeDialog.svelte.js', () => ({ openCvIntake }));
vi.mock('#lib/api.js', () => ({
  api: {
    listCvs,
    listBaseCvs,
    listCvTemplates,
    duplicateCv,
    deleteCv,
    reseedBaseCv,
    cvPdfUrl: (id: string) => `/pdf/${id}`,
  },
  ApiError: class extends Error {},
}));

import CvList from './CvList.svelte';

const base = (id: string, title: string, isDefault = false): CvBaseItem => ({
  id,
  title,
  template_id: 'classic-ats',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-02T10:00:00Z',
  is_default: isDefault,
  hand_edited: false,
});

beforeEach(() => {
  for (const m of [listCvs, listBaseCvs, listCvTemplates, duplicateCv, deleteCv, reseedBaseCv, goto, openCvIntake])
    m.mockReset();
  listCvs.mockResolvedValue([]);
  listCvTemplates.mockResolvedValue([{ id: 'classic-ats', label: 'Classic ATS' }]);
});

describe('CvList base CVs', () => {
  it('renders a card per base with its actions and the default badge', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true), base('b', 'Data')]);
    render(CvList);
    await screen.findByText('Backend');
    expect(screen.getByText('Data')).toBeTruthy();
    expect(screen.getAllByText('Default')).toHaveLength(1);
    expect(screen.getAllByText('Edit')).toHaveLength(2);
    expect(screen.getAllByText('Tailor CV to a job')).toHaveLength(2);
    expect(screen.getAllByLabelText('Duplicate')).toHaveLength(2);
    expect(await screen.findAllByText(/Template: Classic ATS/)).toHaveLength(2);
  });

  it('opens the intake for the base whose card was used', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true), base('b', 'Data')]);
    render(CvList);
    await screen.findByText('Data');
    await fireEvent.click(screen.getAllByText('Tailor CV to a job')[1] as HTMLElement);
    expect(openCvIntake).toHaveBeenCalledWith('b');
  });

  it('duplicates and opens the copy in the editor', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true)]);
    duplicateCv.mockResolvedValue({ id: 'copy-1' });
    render(CvList);
    await fireEvent.click(await screen.findByLabelText('Duplicate'));
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/base/copy-1'));
    expect(duplicateCv).toHaveBeenCalledWith('a');
  });

  it('offers to create a base from the CV when there is none', async () => {
    listBaseCvs.mockResolvedValue([]);
    reseedBaseCv.mockResolvedValue({ id: 'new-1' });
    render(CvList);
    await fireEvent.click(await screen.findByText('Create from my CV'));
    await waitFor(() => expect(goto).toHaveBeenCalledWith('/tailor/base/new-1'));
  });

  it('still lists the tailored copies below the base CVs, each deletable on its own', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true)]);
    listCvs.mockResolvedValue([
      {
        id: 't1',
        title: 'T',
        template_id: 'classic-ats',
        created_at: '2026-10-01T10:00:00Z',
        updated_at: '2026-10-02T10:00:00Z',
        job_slug: 'sre-acme',
        job_title: 'SRE',
        job_company: 'Acme',
        agent_session_id: '',
      },
    ]);
    render(CvList);
    expect(await screen.findByText('Base CVs')).toBeTruthy();
    expect(await screen.findByText('SRE')).toBeTruthy();
    expect(screen.getByText('Tailored CVs')).toBeTruthy();
    expect(screen.getAllByLabelText('Delete')).toHaveLength(2);
  });

  it('keeps the tailored CVs when only the base list fails to load', async () => {
    listBaseCvs.mockRejectedValue(new Error('boom'));
    listCvs.mockResolvedValue([
      {
        id: 't1',
        title: 'T',
        template_id: 'classic-ats',
        created_at: '2026-10-01T10:00:00Z',
        updated_at: '2026-10-02T10:00:00Z',
        job_slug: 'sre-acme',
        job_title: 'SRE',
        job_company: 'Acme',
        agent_session_id: '',
      },
    ]);
    render(CvList);
    expect(await screen.findByText('SRE')).toBeTruthy();
    expect(await screen.findByText('Could not load your base CVs.')).toBeTruthy();
    // A list we could not read is not an empty one.
    expect(screen.queryByText('No base CV yet')).toBeNull();
  });

  it('keeps the base CVs when only the tailored list fails to load', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true)]);
    listCvs.mockRejectedValue(new Error('boom'));
    render(CvList);
    expect(await screen.findByText('Backend')).toBeTruthy();
    expect(screen.getByText('Edit')).toBeTruthy();
    expect(await screen.findByText(/Template: Classic ATS/)).toBeTruthy();
  });

  it('shows the base CVs while the tailored list is still loading', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true)]);
    listCvs.mockReturnValue(new Promise(() => {}));
    render(CvList);
    expect(await screen.findByText('Backend')).toBeTruthy();
    expect(screen.getByText('Edit')).toBeTruthy();
  });

  it('shows a tailored-list failure while the base list is still loading', async () => {
    listBaseCvs.mockReturnValue(new Promise(() => {}));
    listCvs.mockRejectedValue(new Error('boom'));
    render(CvList);
    expect(await screen.findByText('Could not load your CVs.')).toBeTruthy();
  });

  it('offers to create a base when there is none, even if the tailored list fails', async () => {
    listBaseCvs.mockResolvedValue([]);
    listCvs.mockRejectedValue(new Error('boom'));
    render(CvList);
    expect(await screen.findByText('No base CV yet')).toBeTruthy();
  });
});
