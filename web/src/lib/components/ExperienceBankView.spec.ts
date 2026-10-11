import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { CompanyListItem, ExperienceBank, ResumeMeta } from '$lib/types';
import { RESUME_POLL_BUDGET_MS } from '$lib/onboardingResumeWait';
import ExperienceBankView from './ExperienceBankView.svelte';

// The "New experience" add-job form gains a company-catalogue autocomplete (still
// accepting free text) and an "I currently work here" checkbox — see openspec change
// experience-company-picker-present-checkbox.

const { getExperience, listCompanies, createExperienceEmployment, getResume, retryResumeExtract } =
  vi.hoisted(() => ({
    getExperience: vi.fn(),
    listCompanies: vi.fn(),
    createExperienceEmployment: vi.fn(),
    getResume: vi.fn(),
    retryResumeExtract: vi.fn(),
  }));

vi.mock('$lib/api', () => ({
  api: { getExperience, listCompanies, createExperienceEmployment, getResume, retryResumeExtract },
}));

// AssistantChat (mounted unconditionally, just hidden, inside ExperienceAssistantPanel)
// boots a real conversation on mount — irrelevant to the add-job form and not worth
// dragging its own API surface into this test.
vi.mock('$lib/components/ExperienceAssistantPanel.svelte', async () => {
  const stub = await import('../../../vitest-stubs/EmptyComponent.svelte');
  return { default: stub.default };
});

const ringCentral: CompanyListItem = {
  slug: 'ringcentral',
  name: 'RingCentral',
  job_count: 57,
  feedback_count: 0,
  feedback_rating_avg: null,
};

const bankWithOneJob: ExperienceBank = {
  employments: [
    {
      id: 'e1',
      kind: 'job',
      company: 'Acme',
      role: 'Engineer',
      atoms: [{ id: 'a1', claim: 'Shipped things', provenance: 'manual' }],
    },
  ],
  unplaced: [],
};

const resumeOk: ResumeMeta = {
  enabled: true,
  present: true,
  uploaded_at: '2026-10-01T00:00:00Z',
  structured: null,
  parse_status: 'ok',
};

beforeEach(() => {
  getExperience.mockReset().mockResolvedValue(bankWithOneJob);
  listCompanies.mockReset().mockResolvedValue({ items: [ringCentral], hasMore: false });
  createExperienceEmployment.mockReset().mockResolvedValue(undefined);
  getResume.mockReset().mockResolvedValue(resumeOk);
  retryResumeExtract.mockReset().mockResolvedValue(undefined);
});

async function openAddJobForm() {
  render(ExperienceBankView, { props: {} });
  await fireEvent.click(await screen.findByRole('button', { name: 'Add experience' }));
}

describe('ExperienceBankView add-job form', () => {
  it('fills the Company field with the canonical name when a catalogue suggestion is picked', async () => {
    await openAddJobForm();

    await fireEvent.input(screen.getByLabelText('Company'), { target: { value: 'ring' } });
    const option = await screen.findByRole('option', { name: /RingCentral/ });
    await fireEvent.mouseDown(option);

    expect(screen.getByLabelText<HTMLInputElement>('Company').value).toBe('RingCentral');
  });

  it('saves a company that matches no catalogue suggestion as free text', async () => {
    await openAddJobForm();

    await fireEvent.input(screen.getByLabelText('Company'), {
      target: { value: 'A tiny startup nobody crawls' },
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(createExperienceEmployment).toHaveBeenCalledWith(
        expect.objectContaining({ company: 'A tiny startup nobody crawls' }),
      ),
    );
  });

  it('hides the End date and saves as current when "I currently work here" is checked', async () => {
    await openAddJobForm();

    await fireEvent.input(screen.getByLabelText('Company'), { target: { value: 'Acme' } });
    await fireEvent.click(screen.getByLabelText('I currently work here'));

    expect(screen.queryByPlaceholderText('End')).toBeNull();

    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(createExperienceEmployment).toHaveBeenCalledWith(
        expect.objectContaining({ current: true, end: undefined }),
      ),
    );
  });

  it('saves as not current when left unchecked', async () => {
    await openAddJobForm();

    await fireEvent.input(screen.getByLabelText('Company'), { target: { value: 'Acme' } });
    expect(screen.getByPlaceholderText('End')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(createExperienceEmployment).toHaveBeenCalledWith(
        expect.objectContaining({ current: false }),
      ),
    );
  });
});

describe('ExperienceBankView résumé-extraction banner', () => {
  it('shows a retry banner when the résumé failed to parse', async () => {
    getResume.mockResolvedValue({
      enabled: true,
      present: true,
      uploaded_at: '2026-10-01T00:00:00Z',
      structured: null,
      parse_status: 'failed',
      parse_detail: 'extract failed',
    } satisfies ResumeMeta);

    render(ExperienceBankView, { props: {} });

    expect(await screen.findByRole('button', { name: /try again/i })).toBeTruthy();
  });

  it('shows no banner when the résumé parsed fine', async () => {
    render(ExperienceBankView, { props: {} });

    await screen.findByText('Acme');
    expect(screen.queryByRole('button', { name: /try again/i })).toBeNull();
  });

  it('shows no banner when no résumé was ever uploaded', async () => {
    getResume.mockResolvedValue({
      enabled: true,
      present: false,
      uploaded_at: null,
      structured: null,
    } satisfies ResumeMeta);

    render(ExperienceBankView, { props: {} });

    await screen.findByText('Acme');
    expect(screen.queryByRole('button', { name: /try again/i })).toBeNull();
  });

  it('retrying calls the retry endpoint and reloads the bank', async () => {
    getResume.mockResolvedValue({
      enabled: true,
      present: true,
      uploaded_at: '2026-10-01T00:00:00Z',
      structured: null,
      parse_status: 'failed',
      parse_detail: 'extract failed',
    } satisfies ResumeMeta);

    render(ExperienceBankView, { props: {} });
    const retry = await screen.findByRole('button', { name: /try again/i });
    getExperience.mockClear();

    await fireEvent.click(retry);

    await waitFor(() => expect(retryResumeExtract).toHaveBeenCalledTimes(1));
  });

  it('refreshes the bank and clears the banner once the poll sees the retry land', async () => {
    getResume.mockResolvedValueOnce({
      enabled: true,
      present: true,
      uploaded_at: '2026-10-01T00:00:00Z',
      structured: null,
      parse_status: 'failed',
      parse_detail: 'extract failed',
    } satisfies ResumeMeta);

    vi.useFakeTimers();
    try {
      render(ExperienceBankView, { props: {} });
      const retry = await screen.findByRole('button', { name: /try again/i });

      // Every read after the retry click reports success — the poll should accept the
      // very first one it makes, not keep asking.
      getResume.mockResolvedValue({
        enabled: true,
        present: true,
        uploaded_at: '2026-10-01T00:00:00Z',
        structured: null,
        parse_status: 'ok',
      } satisfies ResumeMeta);
      getExperience.mockClear();

      await fireEvent.click(retry);
      expect(screen.getByText(/reading your résumé again/i)).toBeTruthy();

      await vi.advanceTimersByTimeAsync(RESUME_POLL_BUDGET_MS);

      expect(getExperience).toHaveBeenCalled();
      expect(screen.queryByRole('button', { name: /try again/i })).toBeNull();
      expect(screen.queryByText(/reading your résumé again/i)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('stops polling without claiming failure when the retry is still pending at the poll budget', async () => {
    getResume.mockResolvedValueOnce({
      enabled: true,
      present: true,
      uploaded_at: '2026-10-01T00:00:00Z',
      structured: null,
      parse_status: 'failed',
      parse_detail: 'extract failed',
    } satisfies ResumeMeta);

    vi.useFakeTimers();
    try {
      render(ExperienceBankView, { props: {} });
      const retry = await screen.findByRole('button', { name: /try again/i });

      // Every read after the retry click still reports pending — a slow extraction, not a
      // confirmed failure.
      getResume.mockResolvedValue({
        enabled: true,
        present: true,
        uploaded_at: '2026-10-01T00:00:00Z',
        structured: null,
        parse_status: 'pending',
      } satisfies ResumeMeta);

      await fireEvent.click(retry);
      await vi.advanceTimersByTimeAsync(RESUME_POLL_BUDGET_MS);

      // Giving up is not the same as failing (onboardingResumeWait.ts's own documented
      // reasoning): the banner must not come back claiming a failure nothing confirmed.
      expect(screen.queryByRole('button', { name: /try again/i })).toBeNull();
      expect(screen.queryByText(/reading your résumé again/i)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});

afterEach(() => {
  vi.useRealTimers();
});
