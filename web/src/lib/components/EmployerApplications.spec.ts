import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EmployerApplication } from '#lib/types.js';
import EmployerApplications from './EmployerApplications.svelte';

const { listEmployerApplications, listEmployerVacancies, moveEmployerApplication } = vi.hoisted(() => ({
  listEmployerApplications: vi.fn(),
  listEmployerVacancies: vi.fn(),
  moveEmployerApplication: vi.fn(),
}));

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/api.js', () => ({
  api: {
    listEmployerApplications,
    listEmployerVacancies,
    moveEmployerApplication,
    employerApplicationCVUrl: (id: number) => `/api/v1/employer/applications/${id}/cv`,
  },
  ApiError: class extends Error {},
}));

function application(over: Partial<EmployerApplication> = {}): EmployerApplication {
  return {
    id: 1,
    job_slug: 'acme-platform',
    job_title: 'Platform Engineer',
    candidate_email: 'c@example.test',
    cv_kind: 'tailored',
    has_cv: true,
    note: 'Happy to talk.',
    answers: [{ id: 'q1', label: 'EU work permit?', type: 'boolean', answer: 'true' }],
    stage: 'new',
    sent_at: '2026-11-10T10:00:00Z',
    ...over,
  };
}

function page(apps: EmployerApplication[], counts: Record<string, number>) {
  return { items: apps, total: apps.length, hasMore: false, stageCounts: counts };
}

describe('EmployerApplications', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listEmployerVacancies.mockResolvedValue([]);
  });

  it('lists applications with every stage counted', async () => {
    listEmployerApplications.mockResolvedValue(page([application()], { new: 1, rejected: 2 }));
    render(EmployerApplications);

    expect(await screen.findByText('c@example.test')).toBeTruthy();
    const stages = screen.getByRole('group', { name: 'Stage' });
    expect(within(stages).getByRole('button', { name: /All\s*3/ })).toBeTruthy();
    expect(within(stages).getByRole('button', { name: /Rejected\s*2/ })).toBeTruthy();
  });

  it('opens an application with its answers and CV, and asks for a message only where a letter goes', async () => {
    listEmployerApplications.mockResolvedValue(page([application()], { new: 1 }));
    render(EmployerApplications);

    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    expect(screen.getByText('EU work permit?')).toBeTruthy();
    expect(screen.getByText('Yes')).toBeTruthy();
    expect(screen.getByText('Happy to talk.')).toBeTruthy();
    expect(screen.getByRole('link', { name: /Open CV/ }).getAttribute('href')).toBe(
      '/api/v1/employer/applications/1/cv',
    );

    const select = screen.getByRole('combobox', { name: 'Stage' });
    await fireEvent.change(select, { target: { value: 'reviewing' } });
    expect(screen.queryByLabelText(/Message to the candidate/)).toBeNull();
    expect(screen.getByText(/no email is sent/)).toBeTruthy();

    await fireEvent.change(select, { target: { value: 'rejected' } });
    expect(screen.getByLabelText(/Message to the candidate/)).toBeTruthy();
    expect(screen.getByText(/candidate is emailed/)).toBeTruthy();
  });

  it('moves an application and says whether the candidate was emailed', async () => {
    listEmployerApplications.mockResolvedValue(page([application()], { new: 1 }));
    moveEmployerApplication.mockResolvedValue({
      application: application({ stage: 'rejected', stage_changed_at: '2026-11-12T10:00:00Z' }),
      candidate_notified: true,
    });
    render(EmployerApplications);

    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    await fireEvent.change(screen.getByRole('combobox', { name: 'Stage' }), { target: { value: 'rejected' } });
    await fireEvent.input(screen.getByLabelText(/Message to the candidate/), {
      target: { value: 'Thanks for your time.' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Move' }));

    expect(moveEmployerApplication).toHaveBeenCalledWith(1, 'rejected', 'Thanks for your time.');
    expect(await screen.findByText('c@example.test: Moved. The candidate was emailed.')).toBeTruthy();
    const stages = screen.getByRole('group', { name: 'Stage' });
    expect(within(stages).getByRole('button', { name: /Rejected\s*1/ })).toBeTruthy();
  });

  it('says when a CV reached the employer by email only', async () => {
    listEmployerApplications.mockResolvedValue(page([application({ has_cv: false })], { new: 1 }));
    render(EmployerApplications);

    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    expect(screen.getByText('This CV reached you by email only.')).toBeTruthy();
    expect(screen.queryByRole('link', { name: /Open CV/ })).toBeNull();
  });

  it('keeps the outcome visible when the moved application leaves the filtered list', async () => {
    listEmployerApplications
      .mockResolvedValueOnce(page([application()], { new: 1 }))
      .mockResolvedValueOnce(page([application()], { new: 1 }))
      .mockResolvedValue(page([], { rejected: 1 }));
    moveEmployerApplication.mockResolvedValue({
      application: application({ stage: 'rejected' }),
      candidate_notified: false,
    });
    render(EmployerApplications);

    await fireEvent.click(within(screen.getByRole('group', { name: 'Stage' })).getByRole('button', { name: /New/ }));
    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    await fireEvent.change(screen.getByRole('combobox', { name: 'Stage' }), { target: { value: 'rejected' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Move' }));

    const status = await screen.findByRole('status');
    expect(status.textContent).toMatch(/c@example\.test: Moved, but the email to the candidate failed/);
  });

  it('leaves the draft in another open application alone when an earlier move completes', async () => {
    let resolveMove!: (v: unknown) => void;
    listEmployerApplications.mockResolvedValue(
      page([application(), application({ id: 2, candidate_email: 'd@example.test' })], { new: 2 }),
    );
    moveEmployerApplication.mockImplementation(() => new Promise((r) => (resolveMove = r)));
    render(EmployerApplications);

    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    await fireEvent.change(screen.getByRole('combobox', { name: 'Stage' }), { target: { value: 'rejected' } });
    await fireEvent.input(screen.getByLabelText(/Message to the candidate/), { target: { value: 'For A' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Move' }));

    // Open B and start a message while A's move is still in flight.
    await fireEvent.click(screen.getByRole('button', { name: /d@example\.test/ }));
    await fireEvent.change(screen.getByRole('combobox', { name: 'Stage' }), { target: { value: 'rejected' } });
    await fireEvent.input(screen.getByLabelText(/Message to the candidate/), { target: { value: 'For B' } });

    resolveMove({ application: application({ stage: 'rejected' }), candidate_notified: true });
    expect(await screen.findByText(/c@example\.test: Moved/)).toBeTruthy();
    expect((screen.getByLabelText(/Message to the candidate/) as HTMLTextAreaElement).value).toBe('For B');
  });
});
