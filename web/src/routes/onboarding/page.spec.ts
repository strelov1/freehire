import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { SvelteMap } from 'svelte/reactivity';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Job, OnboardingParse } from '#lib/types.js';

const session = new SvelteMap<string, number | null>();
vi.mock('$app/env', () => ({ browser: false, PUBLIC_OPEN_WITHIN: '' }));
const pageState = vi.hoisted(() => ({
  data: { locale: 'en' },
  url: new URL('http://localhost/onboarding'),
  state: {},
}));
vi.mock('$app/state', () => ({ page: pageState }));
vi.mock('$app/paths', () => ({
  resolve: (p: string, params?: { slug: string }) => (params ? p.replace('[slug]', params.slug) : p),
  base: '',
  assets: '',
}));
vi.mock('$app/navigation', () => ({ afterNavigate() {}, goto: vi.fn(), invalidateAll: vi.fn() }));
const auth = vi.hoisted(() => ({ completedAt: null as string | null, createdAt: null as string | null }));
vi.mock('#lib/auth.svelte.js', () => ({
  isAuthenticated: () => session.get('id') != null,
  currentUser: () =>
    session.get('id') == null
      ? null
      : {
          id: session.get('id'),
          onboarding_completed_at: auth.completedAt,
          created_at: auth.createdAt ?? new Date().toISOString(),
        },
}));
vi.mock('#lib/analytics.js', () => ({ track() {} }));

const h = vi.hoisted(() => ({
  api: {
    onboardingParseCv: vi.fn(),
    onboardingMatches: vi.fn(),
    facetCounts: vi.fn(),
    updateSurvey: vi.fn(),
    catalogScale: vi.fn(),
    completeOnboarding: vi.fn(),
    putResume: vi.fn(),
    listViewedSlugs: async () => [],
    listSavedSlugs: async () => [],
    listDismissedSlugs: async () => [],
    listAppliedCompanySlugs: async () => [],
  },
  draft: { load: vi.fn(), save: vi.fn(), clear: vi.fn() },
  profileStore: {
    profile: null as unknown,
    loaded: true,
    ensureLoaded: vi.fn(),
    save: vi.fn(),
  },
  savedSearches: { items: [], ensureLoaded: vi.fn() },
  enableProfileAlert: vi.fn(),
  gotoPath: vi.fn(),
}));
vi.mock('#lib/api.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('#lib/api.js')>()),
  api: h.api,
}));
vi.mock('#lib/onboardingDraft.js', async (importOriginal) => ({
  ...(await importOriginal<typeof import('#lib/onboardingDraft.js')>()),
  onboardingDraft: h.draft,
}));
vi.mock('#lib/profile.svelte.js', () => ({ profileStore: h.profileStore }));
vi.mock('#lib/savedSearches.svelte.js', () => ({ savedSearches: h.savedSearches }));
vi.mock('#lib/profileAlert.js', () => ({ enableProfileAlert: h.enableProfileAlert }));
vi.mock('#lib/profileAlertSync.js', () => ({ syncProfileAlert: async () => {} }));
vi.mock('#lib/resume.svelte.js', () => ({ resumeStore: { noteUpload() {} } }));
vi.mock('#lib/gotoPath.js', () => ({ gotoPath: h.gotoPath }));

import { ApiError } from '#lib/api.js';
import { onboardingGate } from '#lib/onboardingGate.svelte.js';
import Page from './+page.svelte';

const parse: OnboardingParse = {
  structured_available: true,
  first_name: 'Ana',
  headline: 'Backend engineer',
  country: 'br',
  total_years: 9,
  companies: ['Nubank', 'Stone'],
  skills: ['go', 'kubernetes'],
  categories: ['backend'],
  seniority: 'senior',
  report: {
    overall: 64,
    potential: 88,
    reviewed: false,
    strong_keywords: [],
    recommended_keywords: [],
    categories: [
      {
        id: 'impact',
        label: 'Impact',
        score: 5,
        max: 20,
        items: [{ points: 0, text: 'Quantify results', status: 'fail' }],
      },
    ],
  },
};

const job = (n: number, company = 'Acme') =>
  ({
    public_slug: `job-${n}`,
    title: `Platform Engineer ${n}`,
    company,
    work_mode: 'remote',
    countries: ['br'],
    skills: ['go', 'kubernetes'],
  }) as unknown as Job;

// Seven top matches at five employers, out of 526 the search found.
const COMPANIES = ['Magnit Tech', 'Placements24', 'Magnit Tech', 'Acme', 'Globex', 'Initech', 'Umbrella'];
const topMatches = COMPANIES.map((c, i) => ({ job: job(i + 1, c), match_percent: 92 - i }));

const pdf = () => new File(['%PDF'], 'cv.pdf', { type: 'application/pdf' });

async function dropCv() {
  const input = document.querySelector('input[type=file]') as HTMLInputElement;
  await fireEvent.change(input, { target: { files: [pdf()] } });
}

async function answerAll(challenges: string[] = []) {
  await fireEvent.click(await screen.findByRole('button', { name: 'Remote' }));
  await fireEvent.click(await screen.findByRole('button', { name: /^Senior Backend/ }));
  await screen.findByRole('heading', { name: "What's in your way right now?" });
  for (const label of challenges) await fireEvent.click(screen.getByRole('button', { name: label }));
  await fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
}

beforeEach(() => {
  session.set('id', null);
  vi.clearAllMocks();
  auth.completedAt = null;
  auth.createdAt = null;
  pageState.url = new URL('http://localhost/onboarding');
  onboardingGate.reset();
  h.api.onboardingParseCv.mockResolvedValue(parse);
  h.api.facetCounts.mockResolvedValue({ total: 900, facets: { category: { backend: 600, devops: 300 } }, stats: {} });
  h.api.updateSurvey.mockResolvedValue({});
  h.api.onboardingMatches.mockResolvedValue({ data: topMatches, meta: { total: 526, limit: 10, offset: 0 } });
  h.api.catalogScale.mockResolvedValue({
    open_jobs: 100,
    unique_open_jobs: 90,
    companies: 10,
    sources: 0,
    ats_platforms: 5,
    telegram_channels: 0,
    computed_at: '',
    exact: true,
  });
  h.api.completeOnboarding.mockResolvedValue(undefined);
  h.api.putResume.mockResolvedValue({ enabled: true, present: true });
  h.draft.load.mockResolvedValue(null);
  h.draft.save.mockResolvedValue(undefined);
  h.draft.clear.mockResolvedValue(undefined);
  h.profileStore.profile = null;
  h.profileStore.save.mockImplementation(async (specializations: string[], skills: string[], seniorities: string[]) => {
    const saved = { specializations, skills, seniorities, excluded_skills: [], location_preferences: null };
    h.profileStore.profile = saved;
    return saved;
  });
  h.enableProfileAlert.mockResolvedValue(undefined);
});

describe('/onboarding', () => {
  it('shows an anonymous visitor the CV drop, with no sign-in redirect', async () => {
    render(Page);
    expect(screen.getByRole('heading', { name: 'Start with your CV' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Choose a PDF' })).toBeTruthy();
    await Promise.resolve();
    expect(h.gotoPath).not.toHaveBeenCalled();
  });

  it('offers an existing account a way to sign in from the first step', () => {
    render(Page);
    const link = screen.getByRole('link', { name: 'Sign in' });
    const url = new URL(link.getAttribute('href') ?? '', 'http://x');
    expect(url.pathname).toBe('/signin');
    expect(url.searchParams.get('mode')).toBe('login');
  });

  it('offers the roles the catalogue holds for the CV, with their open jobs', async () => {
    h.api.facetCounts.mockResolvedValue({
      total: 9000,
      facets: { category: { backend: 2019, fullstack: 3411, devops: 0 } },
      stats: {},
    });
    render(Page);
    await dropCv();
    await fireEvent.click(await screen.findByRole('button', { name: 'Remote' }));
    expect(await screen.findByRole('button', { name: /^Senior Full-Stack\s+3,411 jobs$/ })).toBeTruthy();
    expect(screen.getByRole('button', { name: /^Senior Backend\s+2,019 jobs$/ })).toBeTruthy();
    const asked = h.api.facetCounts.mock.calls.at(-1)?.[0] as URLSearchParams;
    expect(asked.get('work_mode')).toBe('remote');
    expect(asked.has('category')).toBe(false);
  });

  it('reads the CV, asks three questions and shows the matching jobs open', async () => {
    render(Page);
    await dropCv();
    expect(await screen.findByRole('heading', { name: "Read Ana's CV" })).toBeTruthy();
    expect(screen.getByText('Nubank and Stone')).toBeTruthy();
    await answerAll();

    expect(await screen.findByText('526 open jobs for your answers')).toBeTruthy();
    const [, query] = h.api.onboardingMatches.mock.calls[0] as [string[], string];
    expect(new URLSearchParams(query).get('work_mode')).toBe('remote');
    expect(new URLSearchParams(query).get('posted_within_days')).toBe('7');
    // Who is hiring: two named, the jobs beyond theirs counted from the total.
    expect(screen.getByText('Magnit Tech, Placements24 and 523 more jobs')).toBeTruthy();
    // The five best, one line each, linking to the posting, with the match the search scored.
    const rows = screen.getAllByRole('link', { name: /^Platform Engineer \d$/ });
    expect(rows).toHaveLength(5);
    expect(rows[0]?.getAttribute('href')).toBe('/jobs/job-1');
    expect(screen.getByText('92%')).toBeTruthy();
    expect(screen.queryByText('Platform Engineer 6')).toBeNull();
    expect(document.querySelector('[class*="blur"]')).toBeNull();
    // All of them, in the feed, under the same search.
    const feed = screen.getByRole('link', { name: /See all 526 in the feed/ });
    expect(feed.getAttribute('href')).toBe(`/jobs?${query}`);
    // The catalogue line leaves out the unmeasured (zero) source count.
    expect(screen.getByText('Searching 90 open jobs across 5 ATS platforms')).toBeTruthy();
    // The ATS scorecard: score, band, what to fix, the full check.
    expect(screen.getByRole('img', { name: 'ATS score 64 out of 100, Fair' })).toBeTruthy();
    expect(screen.getByText('Quantify results')).toBeTruthy();
    expect(screen.getByRole('link', { name: /Check your CV in detail/ }).getAttribute('href')).toBe('/roast');
    // The product introduction is not part of the results any more.
    expect(screen.queryByRole('heading', { name: 'What helps' })).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Keep these matches coming' })).toBeNull();
    // Nothing is parked in the browser until the visitor leaves for the sign-in page.
    expect(h.draft.save).not.toHaveBeenCalled();
  });

  it('asks what is in the way: several picks, "just starting" alone, a note beside "something else"', async () => {
    render(Page);
    await dropCv();
    await fireEvent.click(await screen.findByRole('button', { name: 'Remote' }));
    await fireEvent.click(await screen.findByRole('button', { name: /^Senior Backend/ }));
    const visa = await screen.findByRole('button', { name: 'Getting visa sponsorship' });
    const english = screen.getByRole('button', { name: 'Language requirements' });
    await fireEvent.click(visa);
    await fireEvent.click(english);
    expect(visa.getAttribute('aria-pressed')).toBe('true');
    expect(english.getAttribute('aria-pressed')).toBe('true');
    await fireEvent.click(screen.getByRole('button', { name: "Nothing yet — I'm just starting" }));
    expect(visa.getAttribute('aria-pressed')).toBe('false');
    expect(screen.queryByLabelText('Tell us in your own words')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Something else' }));
    expect(screen.getByLabelText('Tell us in your own words')).toBeTruthy();
  });

  // The tips for these answers live on /my/getting-started, not on the results.
  it('narrows to sponsoring jobs for someone who needs a visa', async () => {
    render(Page);
    await dropCv();
    await answerAll(['Getting visa sponsorship']);
    expect(await screen.findByText('526 open jobs for your answers')).toBeTruthy();
    const [, query] = h.api.onboardingMatches.mock.calls[0] as [string[], string];
    expect(new URLSearchParams(query).get('visa_sponsorship')).toBe('true');
    expect(screen.queryByRole('heading', { name: 'What helps' })).toBeNull();
  });

  it('saves the answers about what is in the way with the account', async () => {
    session.set('id', 7);
    render(Page);
    await dropCv();
    await answerAll(['Getting visa sponsorship']);
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    expect(h.api.updateSurvey).toHaveBeenCalledWith({ biggest_challenges: ['visa_sponsorship'] });
  });

  it('moves focus to the next question after an answer', async () => {
    render(Page);
    await dropCv();
    await fireEvent.click(await screen.findByRole('button', { name: 'Remote' }));
    const next = await screen.findByRole('heading', { name: "What's next for you, Ana?" });
    expect(document.activeElement).toBe(next);
  });

  it('widens an empty week to thirty days and says so', async () => {
    h.api.onboardingMatches
      .mockResolvedValueOnce({ data: [], meta: { total: 0, limit: 10, offset: 0 } })
      .mockResolvedValueOnce({ data: [{ job: job(3), match_percent: 60 }], meta: { total: 1, limit: 10, offset: 0 } });
    render(Page);
    await dropCv();
    await answerAll();
    expect(await screen.findByText('1 open job for your answers')).toBeTruthy();
    expect(screen.getByText('Nothing was posted in the last 7 days, so these are from the last 30.')).toBeTruthy();
    const [, wide] = h.api.onboardingMatches.mock.calls[1] as [string[], string];
    expect(new URLSearchParams(wide).get('posted_within_days')).toBe('30');
  });

  it('says the daily limit is reached and offers sign-in on a 429', async () => {
    h.api.onboardingParseCv.mockRejectedValue(new ApiError(429, "You've hit today's limit for anonymous CV reads."));
    render(Page);
    await dropCv();
    expect(await screen.findByText("You've hit today's limit for anonymous CV reads.")).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Sign in or create an account' })).toBeTruthy();
  });

  it('carries the CV, profile and alert for a signed-in account and lands on the feed', async () => {
    session.set('id', 7);
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    expect(h.api.putResume).toHaveBeenCalled();
    expect(h.profileStore.save.mock.calls[0]?.slice(0, 3)).toEqual([['backend'], ['go', 'kubernetes'], ['senior']]);
    expect(h.enableProfileAlert.mock.calls[0]?.[1]).toBe('email');
    expect(h.api.completeOnboarding).toHaveBeenCalled();
    expect(h.draft.clear).toHaveBeenCalled();
    expect(String(h.gotoPath.mock.calls[0]?.[0])).toMatch(/^\/jobs\?/);
  });

  it('reports the part that did not save and keeps the visitor on the page', async () => {
    session.set('id', 7);
    h.enableProfileAlert.mockRejectedValue(new Error('subscribe failed'));
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    expect(await screen.findByText("Your account is ready, but we couldn't save the email alert.")).toBeTruthy();
    expect(h.api.putResume).toHaveBeenCalled();
    expect(h.profileStore.save).toHaveBeenCalled();
    expect(h.gotoPath).not.toHaveBeenCalled();
  });

  const draftOf = (over: Record<string, unknown> = {}) => ({
    file: new Blob(['%PDF'], { type: 'application/pdf' }),
    fileName: 'cv.pdf',
    parse,
    answers: { where: 'remote', role: 'backend', challenge: [] },
    savedAt: Date.now(),
    ...over,
  });

  // Migration 0207 cleared onboarding_completed_at for accounts that existed then: such an
  // account signing in from the results is not new and must be asked, not overwritten.
  it('asks before saving into an old account that was never marked onboarded', async () => {
    session.set('id', 7);
    auth.createdAt = '2025-06-01T00:00:00Z';
    h.draft.load.mockResolvedValue(draftOf({ pendingSignupAt: Date.now() - 60_000 }));
    render(Page);
    expect(await screen.findByRole('button', { name: /Save and go to my feed/ })).toBeTruthy();
    expect(h.api.putResume).not.toHaveBeenCalled();
    expect(h.profileStore.save).not.toHaveBeenCalled();
  });

  it('carries a draft into the account a sign-up from it just created', async () => {
    session.set('id', 7);
    h.draft.load.mockResolvedValue(draftOf({ pendingSignupAt: Date.now() - 60_000 }));
    render(Page);
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    expect(h.api.putResume).toHaveBeenCalled();
    expect(h.api.onboardingParseCv).not.toHaveBeenCalled();
  });

  // Visitor A left the sign-in page without signing up; B, at the same browser, signs up
  // later. A's draft is read back (and deleted) on A's return, so nothing reaches B.
  it('reads back a sign-up left on the sign-in page once, and keeps nothing for later', async () => {
    h.draft.load.mockResolvedValue(draftOf({ pendingSignupAt: Date.now() - 60_000 }));
    render(Page);
    expect(await screen.findByText(/Ana/)).toBeTruthy();
    expect(h.draft.clear).toHaveBeenCalled();
    expect(h.draft.save).not.toHaveBeenCalled();
    expect(h.api.putResume).not.toHaveBeenCalled();
  });

  // A shared browser: somebody ran the flow signed out, then somebody else signed in.
  it('never carries or shows a draft no sign-up was started from', async () => {
    session.set('id', 7);
    h.draft.load.mockResolvedValue(draftOf());
    render(Page);
    await waitFor(() => expect(h.draft.clear).toHaveBeenCalled());
    expect(h.api.putResume).not.toHaveBeenCalled();
    expect(h.profileStore.save).not.toHaveBeenCalled();
    expect(h.enableProfileAlert).not.toHaveBeenCalled();
    expect(screen.queryByText(/Ana/)).toBeNull();
    expect(screen.getByRole('heading', { name: 'Start with your CV' })).toBeTruthy();
  });

  it('only offers to save a fresh sign-up into an account that existed before', async () => {
    session.set('id', 7);
    auth.completedAt = '2026-01-01T00:00:00Z';
    auth.createdAt = '2025-06-01T00:00:00Z';
    h.draft.load.mockResolvedValue(draftOf({ pendingSignupAt: Date.now() - 60_000 }));
    render(Page);
    expect(await screen.findByRole('button', { name: 'Save and go to my feed' })).toBeTruthy();
    expect(h.api.putResume).not.toHaveBeenCalled();
  });

  it('keeps every draft it saves storable — IndexedDB refuses a reactive proxy', async () => {
    render(Page);
    await dropCv();
    await answerAll(['Getting visa sponsorship']);
    await fireEvent.click(await screen.findByRole('button', { name: 'Save my matches' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    // structuredClone is the algorithm IndexedDB stores with: a Svelte $state proxy in the
    // draft makes it throw DataCloneError, and the save (with its sign-up mark) is lost.
    for (const [draft] of h.draft.save.mock.calls) expect(() => structuredClone(draft)).not.toThrow();
    const marked = h.draft.save.mock.calls.at(-1)?.[0] as { answers: { challenge?: string[] } };
    expect(marked.answers.challenge).toEqual(['visa_sponsorship']);
  });

  it('marks the draft and sends the visitor to the sign-in page, which brings them back', async () => {
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save my matches' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    const marked = h.draft.save.mock.calls.at(-1)?.[0] as { pendingSignupAt?: number };
    expect(marked.pendingSignupAt).toBeTypeOf('number');
    const target = new URL(h.gotoPath.mock.calls.at(-1)?.[0] as string, 'http://x');
    expect(target.pathname).toBe('/signin');
    expect(target.searchParams.get('returnTo')).toBe('/onboarding');
    expect(h.api.putResume).not.toHaveBeenCalled();
  });

  it('clears the draft and dismisses the gate once every step was tried, even with a failure', async () => {
    session.set('id', 7);
    h.enableProfileAlert.mockRejectedValue(new Error('subscribe failed'));
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    expect(await screen.findByText(/couldn't save the email alert/)).toBeTruthy();
    expect(h.draft.clear).toHaveBeenCalled();
    expect(onboardingGate.dismissed).toBe(true);
    expect(screen.queryByRole('button', { name: /Change answers/ })).toBeNull();
  });

  it('reports the CV as not saved when storing it fails', async () => {
    session.set('id', 7);
    h.api.putResume.mockRejectedValue(new ApiError(501, 'résumé storage is not available'));
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    expect(await screen.findByText("Your account is ready, but we couldn't save your CV.")).toBeTruthy();
    expect(h.gotoPath).not.toHaveBeenCalled();
  });

  it('lets a signed-in visitor skip to where they were headed', async () => {
    session.set('id', 7);
    pageState.url = new URL('http://localhost/onboarding?returnTo=%2Fjobs%2Fjob-1');
    render(Page);
    await fireEvent.click(screen.getByRole('button', { name: 'Skip for now' }));
    expect(h.gotoPath).toHaveBeenCalledWith('/jobs/job-1');
  });

  it('goes back where the visitor was headed after saving', async () => {
    session.set('id', 7);
    pageState.url = new URL('http://localhost/onboarding?returnTo=%2Fjobs%2Fjob-1');
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalledWith('/jobs/job-1'));
  });

  it('keeps where the visitor was headed through the sign-in page', async () => {
    pageState.url = new URL('http://localhost/onboarding?returnTo=%2Fjobs%2Fjob-1');
    render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save my matches' }));
    await waitFor(() => expect(h.gotoPath).toHaveBeenCalled());
    const signin = new URL(h.gotoPath.mock.calls.at(-1)?.[0] as string, 'http://x');
    const back = new URL(signin.searchParams.get('returnTo') ?? '', 'http://x');
    expect(back.pathname).toBe('/onboarding');
    expect(back.searchParams.get('returnTo')).toBe('/jobs/job-1');
  });

  it('stays where the visitor went if they leave while saving', async () => {
    session.set('id', 7);
    let finish: (v: unknown) => void = () => {};
    h.api.putResume.mockReturnValue(new Promise((r) => (finish = r)));
    const { unmount } = render(Page);
    await dropCv();
    await answerAll();
    await fireEvent.click(await screen.findByRole('button', { name: 'Save and go to my feed' }));
    unmount();
    finish({ enabled: true, present: true });
    await new Promise((r) => setTimeout(r, 20));
    expect(h.gotoPath).not.toHaveBeenCalled();
  });

  it('shows no match figures for a CV with no recognised skills', async () => {
    h.api.onboardingParseCv.mockResolvedValue({ ...parse, skills: [] });
    render(Page);
    await dropCv();
    await answerAll();
    expect(await screen.findByText('526 open jobs for your answers')).toBeTruthy();
    expect(screen.queryByText('92%')).toBeNull();
    expect(screen.getByText(/Matching jobs to your answers/)).toBeTruthy();
  });
});
