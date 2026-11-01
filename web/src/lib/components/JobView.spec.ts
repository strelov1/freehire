import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { tick } from 'svelte';
import type { DirectApplyForm, Job, UserJob, UserProfile } from '#lib/types.js';
import { ensureAppliedCompaniesLoaded, hasAppliedToCompany, markAppliedToCompany } from '#lib/appliedCompanies.svelte.js';
import { resetUserStores } from '#lib/userResource.svelte.js';
import { profileStore } from '#lib/profile.svelte.js';
import JobView from './JobView.svelte';
import JobRow from './JobRow.svelte';

vi.mock('$app/env', () => ({ browser: true }));
vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs/acme-job'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
const session = vi.hoisted(() => ({ userID: 7 as number | null }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => session.userID !== null,
  currentUser: () => session.userID === null ? null : { id: session.userID },
}));
vi.mock('#lib/analytics.js', () => ({ track() {} }));

const { markJobApplied, listAppliedCompanySlugs, listSavedSlugs, getProfile, getDirectApplyForm } = vi.hoisted(() => ({
  markJobApplied: vi.fn(),
  listAppliedCompanySlugs: vi.fn(),
  listSavedSlugs: vi.fn(),
  getProfile: vi.fn(),
  getDirectApplyForm: vi.fn(),
}));
vi.mock('#lib/api.js', () => ({
  ApiError: class extends Error {},
  RESUME_MAX_MB: 10,
  api: {
    markJobApplied,
    listAppliedCompanySlugs,
    listSavedSlugs,
    getProfile,
    getDirectApplyForm,
    countThreads: async () => 0,
    myPlan: async () => null,
    recordJobView: async () => null,
    getMatchAnalysis: async () => null,
  },
}));

const job: Job = {
  public_slug: 'acme-job', source: 'greenhouse', manually_added: false, external_id: '1',
  url: 'https://example.test/apply', title: 'Engineer', company: 'Acme', company_slug: 'acme',
  location: '', description: 'Build things.', countries: [], regions: [], cities: [], metros: [],
  skills: [], collections: [], perks: [], enrichment: {}, enrichment_version: 0,
  view_count: 0, applied_count: 0, upvote_count: 0, downvote_count: 0, my_vote: 0,
};

beforeEach(() => {
  session.userID = 7;
  vi.stubGlobal('IntersectionObserver', class { observe() {} disconnect() {} });
  resetUserStores();
  markJobApplied.mockReset();
  listAppliedCompanySlugs.mockReset().mockResolvedValue([]);
  listSavedSlugs.mockReset().mockResolvedValue([]);
  getProfile.mockReset().mockResolvedValue(null);
  const form: DirectApplyForm = {
    company_name: 'Acme', questions: [], open: true, applied: false, prefill: {},
    cvs: { tailored: null, has_resume: true },
  };
  getDirectApplyForm.mockReset().mockResolvedValue(form);
});

describe('JobView deferred user data', () => {
  it.each(['applied companies', 'profile'] as const)('keeps a draft open after %s loads', async (resource) => {
    const applied = Promise.withResolvers<string[]>();
    const profile = Promise.withResolvers<UserProfile | null>();
    if (resource === 'applied companies') listAppliedCompanySlugs.mockReturnValue(applied.promise);
    else getProfile.mockReturnValue(profile.promise);
    const view = render(JobView, { job: { ...job, direct_apply: true } });
    // Let the other resource settle before opening the form.
    if (resource === 'applied companies') await profileStore.ensureLoaded();
    else await ensureAppliedCompaniesLoaded();
    await tick();
    expect(listAppliedCompanySlugs).toHaveBeenCalled();
    expect(getProfile).toHaveBeenCalled();
    const [applyButton] = screen.getAllByRole('button', { name: /^Apply/ });
    if (!applyButton) throw new Error('Missing direct application button');
    await fireEvent.click(applyButton);
    const note = await screen.findByLabelText<HTMLTextAreaElement>('A note to the employer (optional)', {}, { timeout: 10_000 });
    await fireEvent.input(note, { target: { value: 'My application draft' } });
    if (resource === 'applied companies') {
      applied.resolve([]);
      await ensureAppliedCompaniesLoaded();
    } else {
      profile.resolve(null);
      await profileStore.ensureLoaded();
    }
    await tick();
    expect(screen.queryByRole('dialog', { name: 'Apply: Engineer' })).not.toBeNull();
    expect(screen.getByLabelText<HTMLTextAreaElement>('A note to the employer (optional)').value).toBe('My application draft');
    await view.rerender({ job: { ...job, public_slug: 'another-job', direct_apply: true } });
    expect(screen.queryByRole('dialog', { name: 'Apply: Engineer' })).toBeNull();
  }, 20_000);
});

describe('JobView application completion', () => {
  it('does not mark an employer in a replacement account after a delayed application', async () => {
    const response = Promise.withResolvers<UserJob>();
    markJobApplied.mockReturnValue(response.promise);
    render(JobView, { job });
    const [applyLink] = screen.getAllByRole('link', { name: /^Apply/ });
    if (!applyLink) throw new Error('Missing application link');
    await fireEvent.click(applyLink);
    await fireEvent.click(screen.getByRole('button', { name: 'Yes, save' }));
    session.userID = 8;
    resetUserStores();
    response.resolve({
      job_id: 1, viewed_at: '2026-10-31T12:00:00Z', applied_at: '2026-10-31T12:00:00Z',
      saved_at: null, dismissed_at: null, stage: 'applied', notes: null,
    });
    await response.promise;
    await tick();
    expect(hasAppliedToCompany('acme')).toBe(false);
  }, 20_000);

  it('marks the submitted employer when navigation happens during the request', async () => {
    const response = Promise.withResolvers<UserJob>();
    markJobApplied.mockReturnValue(response.promise);
    const view = render(JobView, { job });
    const [applyLink] = screen.getAllByRole('link', { name: /^Apply/ });
    if (!applyLink) throw new Error('Missing application link');
    await fireEvent.click(applyLink);
    await fireEvent.click(screen.getByRole('button', { name: 'Yes, save' }));
    await view.rerender({ job: { ...job, public_slug: 'globex-job', company: 'Globex', company_slug: 'globex' } });
    response.resolve({
      job_id: 1, viewed_at: '2026-10-31T12:00:00Z', applied_at: '2026-10-31T12:00:00Z',
      saved_at: null, dismissed_at: null, stage: 'applied', notes: null,
    });
    await waitFor(() => expect(hasAppliedToCompany('acme')).toBe(true));
    expect(hasAppliedToCompany('globex')).toBe(false);
    expect(screen.queryByText('Applied')).toBeNull();
  }, 20_000);
});

it('includes the applied employer in the card link description', () => {
  markAppliedToCompany('acme');
  render(JobRow, { job });
  const link = screen.getByRole('link', { name: 'Engineer at Acme' });
  const description = document.getElementById(link.getAttribute('aria-describedby') ?? '');
  expect(description?.textContent).toContain('Applied to this company');
});

it('shows exact source salary bounds in the job detail summary', () => {
  render(JobView, { job: { ...job, enrichment: {
    salary_min: 2_501, salary_max: 3_000, salary_currency: 'EUR', salary_period: 'month',
  } } });
  expect(screen.getByText('2,501 – 3,000 € / mo')).not.toBeNull();
});

it.each([JobView, JobRow])('shows the application deadline when the source states it', (component) => {
  const datedJob = { ...job, application_deadline: '2026-11-11' };
  const view = render(component, { job: datedJob });
  const deadline = view.container.querySelector('time[datetime="2026-11-11"]');
  expect(deadline?.textContent).toBe('2026-11-11');
  expect(deadline?.parentElement?.textContent).toContain('Apply by');
});

it('shows removal for a saved job when the card loads before the saved cache', async () => {
  listSavedSlugs.mockResolvedValue([job.public_slug]);
  render(JobRow, { job });
  expect(await screen.findByRole('button', { name: 'Remove from saved' })).toBeTruthy();
  expect(listSavedSlugs).toHaveBeenCalledOnce();
});

it('waits for saved state before toggling a card while its initial load is pending', async () => {
  const saved = Promise.withResolvers<string[]>();
  listSavedSlugs.mockReturnValue(saved.promise);
  const unsave = vi.fn().mockResolvedValue(undefined);
  render(JobRow, { job, unsave });
  await fireEvent.click(screen.getByRole('button', { name: 'Save to board' }));
  expect(unsave).not.toHaveBeenCalled();

  saved.resolve([job.public_slug]);
  await waitFor(() => expect(unsave).toHaveBeenCalledWith(job.public_slug));
});
