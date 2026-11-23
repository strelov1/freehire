import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { EmployerApplication } from '#lib/types.js';
import EmployerApplications from './EmployerApplications.svelte';

const {
  listEmployerApplications,
  listEmployerVacancies,
  moveEmployerApplication,
  updateEmployerApplicationWorkspace,
  getEmployerApplicationConversation,
  sendEmployerApplicationMessage,
  markEmployerApplicationRepliesRead,
} = vi.hoisted(() => ({
  listEmployerApplications: vi.fn(),
  listEmployerVacancies: vi.fn(),
  moveEmployerApplication: vi.fn(),
  updateEmployerApplicationWorkspace: vi.fn(),
  getEmployerApplicationConversation: vi.fn(),
  sendEmployerApplicationMessage: vi.fn(),
  markEmployerApplicationRepliesRead: vi.fn(),
}));

vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/') } }));
vi.mock('#lib/api.js', () => ({
  api: {
    listEmployerApplications,
    listEmployerVacancies,
    moveEmployerApplication,
    updateEmployerApplicationWorkspace,
    getEmployerApplicationConversation,
    sendEmployerApplicationMessage,
    markEmployerApplicationRepliesRead,
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

  it('shows a candidate reply and clears the unread badge when acknowledged', async () => {
    listEmployerApplications.mockResolvedValue(page([application({ has_unread_reply: true })], { new: 1 }));
    getEmployerApplicationConversation.mockResolvedValue({
      can_send: true,
      has_more: false,
      messages: [{ id: 4, status: 'received', body: 'Tuesday works for me.', created_at: '2026-11-20T14:00:00Z' }],
    });
    markEmployerApplicationRepliesRead.mockResolvedValue({ has_unread_reply: false });
    render(EmployerApplications);

    expect(await screen.findByText('New reply')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: /c@example\.test/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Conversation' }));
    expect(await screen.findByText('Tuesday works for me.')).toBeTruthy();
    expect(markEmployerApplicationRepliesRead).toHaveBeenCalledWith(1, 4);
    await waitFor(() => expect(screen.queryByText('New reply')).toBeNull());
  });

  it('keeps a failed candidate email in the conversation so the team can retry it', async () => {
    listEmployerApplications.mockResolvedValue(page([application()], { new: 1 }));
    getEmployerApplicationConversation.mockResolvedValue({ can_send: true, has_more: false, messages: [] });
    sendEmployerApplicationMessage.mockImplementation(async (_id: number, body: string, key: string) => ({
      id: 7,
      status: sendEmployerApplicationMessage.mock.calls.length === 1 ? 'failed' : 'sent',
      body,
      request_key: key,
      created_at: '2026-11-20T14:00:00Z',
    }));
    render(EmployerApplications);

    await fireEvent.click(await screen.findByRole('button', { name: /c@example\.test/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Conversation' }));
    await fireEvent.input(await screen.findByLabelText('Write to candidate'), {
      target: { value: 'How about Tuesday?' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Send email' }));
    expect(await screen.findByText('Send could not be confirmed')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Retry send' }));
    expect(await screen.findByText('Sent')).toBeTruthy();
    expect(sendEmployerApplicationMessage.mock.calls[1]).toEqual(sendEmployerApplicationMessage.mock.calls[0]);
    expect(moveEmployerApplication).not.toHaveBeenCalled();
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
  it('keeps a pending notes draft when its application is reopened before saving finishes', async () => {
    let resolveSave!: (saved: EmployerApplication) => void;
    listEmployerApplications.mockResolvedValue(
      page([application({ employer_notes: 'Old notes' }), application({ id: 2, candidate_email: 'd@example.test' })], {
        new: 2,
      }),
    );
    updateEmployerApplicationWorkspace.mockImplementation(() => new Promise((resolve) => (resolveSave = resolve)));
    render(EmployerApplications);

    const first = await screen.findByRole('button', { name: /c@example\.test/ });
    await fireEvent.click(first);
    await fireEvent.input(screen.getByRole('textbox', { name: 'Private team notes' }), {
      target: { value: 'Current draft' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save notes' }));
    await fireEvent.click(screen.getByRole('button', { name: /d@example\.test/ }));
    await fireEvent.click(first);

    const reopened = screen.getByRole<HTMLTextAreaElement>('textbox', { name: 'Private team notes' });
    expect(reopened.value).toBe('Current draft');
    await fireEvent.input(reopened, { target: { value: 'Later edit' } });
    await fireEvent.click(screen.getByRole('button', { name: /d@example\.test/ }));
    await fireEvent.click(first);
    resolveSave(application({ employer_notes: 'Current draft' }));
    expect(await screen.findByText('Notes saved.')).toBeTruthy();
    expect(screen.getByRole<HTMLTextAreaElement>('textbox', { name: 'Private team notes' }).value).toBe('Later edit');
  });
  it('keeps an unsaved note when its request fails while another application is open', async () => {
    let rejectSave!: (error: Error) => void;
    listEmployerApplications.mockResolvedValue(
      page([application(), application({ id: 2, candidate_email: 'd@example.test' })], { new: 2 }),
    );
    updateEmployerApplicationWorkspace.mockImplementation(() => new Promise((_, reject) => (rejectSave = reject)));
    render(EmployerApplications);

    const first = await screen.findByRole('button', { name: /c@example\.test/ });
    await fireEvent.click(first);
    await fireEvent.input(screen.getByRole('textbox', { name: 'Private team notes' }), {
      target: { value: 'Retry me' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save notes' }));
    await fireEvent.click(screen.getByRole('button', { name: /d@example\.test/ }));
    await fireEvent.input(screen.getByRole('textbox', { name: 'Private team notes' }), {
      target: { value: 'Notes for B' },
    });
    rejectSave(new Error('Network error'));
    await waitFor(() =>
      expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Save notes' }).disabled).toBe(false),
    );
    await fireEvent.click(first);
    expect(screen.getByRole<HTMLTextAreaElement>('textbox', { name: 'Private team notes' }).value).toBe('Retry me');
  });
});
