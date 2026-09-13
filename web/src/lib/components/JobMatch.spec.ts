import { render, screen } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job, JobMatchResult } from '$lib/types';
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
    // MatchSummary.svelte (rendered inside the 'ready' branch) fetches this on mount;
    // not the behavior under test.
    getMatchAnalysis: vi.fn(),
    syncProfileAlert: vi.fn(),
  }));

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
vi.mock('$lib/auth.svelte', () => ({ isAuthenticated: () => true }));
vi.mock('$lib/api', () => ({ api: { getJobMatch, getMatchAnalysis } }));
vi.mock('$lib/profileAlertSync', () => ({ syncProfileAlert }));

const job = { public_slug: 'rust-job', skills: ['rust'] } as Job;

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
    render(JobMatch, { props: { job } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /i have it/i }));

    expect(addSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  it('calls syncProfileAlert after avoiding a skill', async () => {
    render(JobMatch, { props: { job } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /^avoid$/i }));

    expect(avoidSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  it('calls syncProfileAlert after un-avoiding a skill', async () => {
    profileStoreMock.profile = { skills: ['go'], excluded_skills: ['rust'] };
    render(JobMatch, { props: { job } });
    await openClaimRow();

    await fireEvent.click(screen.getByRole('button', { name: /stop avoiding/i }));

    expect(unavoidSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);
  });

  it('calls syncProfileAlert again after undoing a claim', async () => {
    render(JobMatch, { props: { job } });
    await openClaimRow();
    await fireEvent.click(screen.getByRole('button', { name: /i have it/i }));
    expect(syncProfileAlert).toHaveBeenCalledTimes(1);

    await fireEvent.click(await screen.findByRole('button', { name: /undo/i }));

    expect(removeSkill).toHaveBeenCalledWith('rust');
    expect(syncProfileAlert).toHaveBeenCalledTimes(2);
  });
});
