import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { CvBaseItem } from '#lib/cv.js';

const { listBaseCvs, searchJobs, goto } = vi.hoisted(() => ({
  listBaseCvs: vi.fn(),
  searchJobs: vi.fn(),
  goto: vi.fn(),
}));

vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/my/cvs') } }));
vi.mock('$app/paths', () => ({ resolve: (route: string, params: Record<string, string>) => route.replace('[slug]', params.slug ?? '') }));
vi.mock('#lib/api.js', () => ({
  api: { listBaseCvs, searchJobs, resolveJd: vi.fn() },
  ApiError: class extends Error {},
}));

import JdIntakeDialog from './JdIntakeDialog.svelte';

const base = (id: string, title: string, isDefault = false): CvBaseItem => ({
  id,
  title,
  template_id: 'classic-ats',
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-02T10:00:00Z',
  is_default: isDefault,
  hand_edited: false,
});

const job = { public_slug: 'sre-acme-1a2b', title: 'SRE', company: 'Acme' };

async function pickJob() {
  await fireEvent.input(screen.getByPlaceholderText('Job title or company…'), { target: { value: 'sre' } });
  const result = await screen.findByText('SRE', undefined, { timeout: 2000 });
  await fireEvent.click(result);
}

beforeEach(() => {
  goto.mockReset();
  listBaseCvs.mockReset();
  searchJobs.mockResolvedValue({ items: [job] });
});

describe('JdIntakeDialog base choice', () => {
  it('asks nothing about the base when there is only one', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Only', true)]);
    render(JdIntakeDialog, { onClose: () => {} });
    await waitFor(() => expect(listBaseCvs).toHaveBeenCalled());
    expect(screen.queryAllByRole('radio')).toHaveLength(0);
    await pickJob();
    expect(goto).toHaveBeenCalledWith('/tailor/sre-acme-1a2b');
  });

  it('shows the picker with two bases and sends the one picked', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true), base('b', 'Data')]);
    render(JdIntakeDialog, { onClose: () => {} });
    const radios = await screen.findAllByRole('radio');
    expect(radios).toHaveLength(2);
    const second = radios.find((r) => (r as HTMLInputElement).value === 'b');
    if (!second) throw new Error('no radio for base b');
    await fireEvent.click(second);
    await pickJob();
    expect(goto).toHaveBeenCalledWith('/tailor/sre-acme-1a2b?base=b');
  });

  it('uses the preselected base without showing a picker', async () => {
    listBaseCvs.mockResolvedValue([base('a', 'Backend', true), base('b', 'Data')]);
    render(JdIntakeDialog, { onClose: () => {}, baseCvId: 'b' });
    await screen.findByText('From your base CV: Data');
    expect(screen.queryAllByRole('radio')).toHaveLength(0);
    await pickJob();
    expect(goto).toHaveBeenCalledWith('/tailor/sre-acme-1a2b?base=b');
  });
});
