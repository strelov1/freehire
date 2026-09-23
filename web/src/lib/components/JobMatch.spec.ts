import { render, screen } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job, JobMatchResult, MatchAnalysisResponse } from '$lib/types';
import JobMatch from './JobMatch.svelte';

// JobMatch pulls in SvelteKit runtime modules this test environment doesn't provide
// (no sveltekit() plugin in the `components` vitest project — see vitest.config.ts).
// None of their real behavior is under test here.
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/jobs/rust-job') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const { addSkill, removeSkill, avoidSkill, unavoidSkill, getJobMatch, getMatchAnalysis, syncProfileAlert } =
  vi.hoisted(() => ({
    addSkill: vi.fn(),
    removeSkill: vi.fn(),
    avoidSkill: vi.fn(),
    unavoidSkill: vi.fn(),
    getJobMatch: vi.fn(),
    // Nothing under this component fetches this any more — the page reads it once and
    // passes it down. Kept on the mocked `api` so a call would be RECORDED rather than
    // throwing, which is what lets the test below name it.
    getMatchAnalysis: vi.fn(),
    syncProfileAlert: vi.fn(),
  }));

const authState = vi.hoisted(() => ({ signedIn: true }));

const profileStoreMock = vi.hoisted(() => ({
  loaded: true,
  profile: { skills: ['go'], excluded_skills: [] as string[] },
  ensureLoaded: vi.fn(),
  addSkill,
  removeSkill,
  avoidSkill,
  unavoidSkill,
}));

vi.mock('$lib/profile.svelte', () => ({ profileStore: profileStoreMock }));
vi.mock('$lib/auth.svelte', () => ({ isAuthenticated: () => authState.signedIn }));
vi.mock('$lib/api', () => ({ api: { getJobMatch, getMatchAnalysis } }));
vi.mock('$lib/profileAlertSync', () => ({ syncProfileAlert }));

const job = { public_slug: 'rust-job', skills: ['rust'] } as Job;
// The guest teaser needs two skills to have a have/missing contrast to draw; with one it
// renders the call-to-action alone, which is not the branch under test.
const twoSkillJob = { public_slug: 'rust-job', skills: ['rust', 'go'] } as Job;

const analysed: MatchAnalysisResponse = {
  has_cv: true,
  stale: false,
  analysis: {
    dimensions: [],
    requirement_match: [],
    hidden_signals: [],
    overall_score: 74,
    verdict: 'Good fit',
    strengths: [],
    gaps: [],
    recommendation: '',
    blockers: [],
  },
};

const matchResult: JobMatchResult = {
  total: 1,
  exact_count: 0,
  adjacent_count: 0,
  coverage_percent: 0,
  matched: [],
  adjacent: [],
  missing: ['rust'],
  blockers: [],
};

beforeEach(() => {
  authState.signedIn = true;
  profileStoreMock.profile = { skills: ['go'], excluded_skills: [] };
  addSkill.mockReset().mockResolvedValue(undefined);
  removeSkill.mockReset().mockResolvedValue(undefined);
  avoidSkill.mockReset().mockResolvedValue(undefined);
  unavoidSkill.mockReset().mockResolvedValue(undefined);
  getJobMatch.mockReset().mockResolvedValue(matchResult);
  getMatchAnalysis.mockReset().mockResolvedValue(null);
  syncProfileAlert.mockReset();
});

async function openClaimRow() {
  const chip = await screen.findByRole('button', { name: /rust/i });
  await fireEvent.click(chip);
}

describe('JobMatch', () => {
  it('calls syncProfileAlert after claiming a missing skill', async () => {
    render(JobMatch, { props: { job, matchAnalysis: null } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /i have it/i }));

    expect(addSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  it('calls syncProfileAlert after avoiding a skill', async () => {
    render(JobMatch, { props: { job, matchAnalysis: null } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /^avoid$/i }));

    expect(avoidSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  it('calls syncProfileAlert after un-avoiding a skill', async () => {
    profileStoreMock.profile = { skills: ['go'], excluded_skills: ['rust'] };
    render(JobMatch, { props: { job, matchAnalysis: null } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /stop avoiding/i }));

    expect(unavoidSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  // The block no longer reads the analysis; the page does, once, for the CTA row and this
  // block together. So the only thing left for this component to get wrong is failing to
  // hand it on — which renders a sidebar that silently never reports a cached verdict.
  it('hands the page-supplied analysis to the summary block', async () => {
    render(JobMatch, { props: { job, matchAnalysis: analysed } });

    expect(await screen.findByText('74%')).toBeTruthy();
    expect(screen.getByText('Good fit')).toBeTruthy();
  });

  // A REGRESSION GUARD, not a driver: it is already green, because the button this branch
  // existed to show a guest left in the same change. What it pins is that the branch stays
  // gone — re-adding the summary block here would show a guest the card belonging to
  // whoever the page last read an analysis for.
  it('renders no fit-analysis section for a guest', () => {
    authState.signedIn = false;
    render(JobMatch, { props: { job: twoSkillJob, matchAnalysis: analysed } });

    expect(screen.queryByLabelText(/fit analysis/i)).toBeNull();
    expect(screen.queryByText('74%')).toBeNull();
  });

  it('calls syncProfileAlert again after undoing a claim', async () => {
    render(JobMatch, { props: { job, matchAnalysis: null } });
    await openClaimRow();
    await fireEvent.click(screen.getByRole('button', { name: /i have it/i }));
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);

    await fireEvent.click(await screen.findByRole('button', { name: /undo/i }));

    expect(removeSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(2);
  });
});
