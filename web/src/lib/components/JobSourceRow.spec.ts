import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import JobSourceRow from './JobSourceRow.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { avoidSource, unavoidSource, syncProfileAlert, promptSignIn } = vi.hoisted(() => ({
  avoidSource: vi.fn(),
  unavoidSource: vi.fn(),
  syncProfileAlert: vi.fn(),
  promptSignIn: vi.fn(),
}));

const profileStoreMock = vi.hoisted(() => ({
  loaded: true,
  profile: { excluded_sources: [] as string[] },
  ensureLoaded: vi.fn(),
  avoidSource,
  unavoidSource,
}));

const authMock = vi.hoisted(() => ({ signedIn: true }));

vi.mock('$lib/profile.svelte', () => ({ profileStore: profileStoreMock }));
vi.mock('$lib/auth.svelte', () => ({ isAuthenticated: () => authMock.signedIn }));
vi.mock('$lib/profileAlertSync', () => ({ syncProfileAlert }));
vi.mock('$lib/signin', () => ({ promptSignIn }));

beforeEach(() => {
  vi.clearAllMocks();
  profileStoreMock.profile = { excluded_sources: [] };
  authMock.signedIn = true;
  avoidSource.mockResolvedValue({});
  unavoidSource.mockResolvedValue({});
});

const props = { source: 'smartrecruiters', jobUrl: 'https://example.com/job/1' };

describe('JobSourceRow', () => {
  it('names the source the way the rest of the app does', () => {
    // The sidebar used to print the raw facet key, so the same source read
    // "smartrecruiters" here and "SmartRecruiters" on the filter panel and /sources.
    render(JobSourceRow, props);

    expect(screen.getByText('SmartRecruiters')).toBeTruthy();
    expect(screen.queryByText('smartrecruiters')).toBeNull();
  });

  it('resolves the logo from the display name, not from the source key', () => {
    // A host or a bare key gets the wrong brand or a 404 — see sourceLogoUrl.
    const { container } = render(JobSourceRow, props);

    expect(container.querySelector('img')?.getAttribute('src')).toContain('SmartRecruiters');
  });

  it('writes the avoid to the profile with the raw source key, and re-syncs the alert', async () => {
    // The key, not the label: the search facet filters on `smartrecruiters`, and the alert
    // query is built from the same list. Sending "SmartRecruiters" would store a value
    // nothing matches.
    render(JobSourceRow, props);

    await fireEvent.click(screen.getByRole('button', { name: /Avoid this source/ }));

    expect(avoidSource).toHaveBeenCalledWith('smartrecruiters');
    // Without this the exclusion never reaches the digest — the alert query is rebuilt
    // here, not on a schedule.
    expect(syncProfileAlert).toHaveBeenCalled();
  });

  it('offers to undo once the source is avoided', async () => {
    profileStoreMock.profile = { excluded_sources: ['smartrecruiters'] };
    render(JobSourceRow, props);

    const undo = screen.getByRole('button', { name: /Stop avoiding/ });
    await fireEvent.click(undo);

    expect(unavoidSource).toHaveBeenCalledWith('smartrecruiters');
    expect(avoidSource).not.toHaveBeenCalled();
  });

  it('reads the stored list case-insensitively', () => {
    // The server lowercases on save; nothing guarantees a caller hands us that casing.
    profileStoreMock.profile = { excluded_sources: ['SmartRecruiters'] };
    render(JobSourceRow, props);

    expect(screen.getByRole('button', { name: /Stop avoiding/ })).toBeTruthy();
  });

  it('says plainly that the scope is every posting, not this one', () => {
    // Avoiding here edits the PROFILE. A control that reads as "hide this job" and instead
    // hides thousands is the kind of surprise that gets undone by deleting the account.
    render(JobSourceRow, props);

    const button = screen.getByRole('button', { name: /Avoid this source/ });
    expect(button.getAttribute('title')).toMatch(/every SmartRecruiters posting/);
  });

  it('shows no control at all to a signed-out reader', () => {
    authMock.signedIn = false;
    render(JobSourceRow, props);

    expect(screen.queryByRole('button')).toBeNull();
    // The source itself still renders — provenance is public.
    expect(screen.getByText('SmartRecruiters')).toBeTruthy();
  });

  it('reports a failed write instead of silently leaving the mark unchanged', async () => {
    avoidSource.mockRejectedValue(new Error('nope'));
    render(JobSourceRow, props);

    await fireEvent.click(screen.getByRole('button', { name: /Avoid this source/ }));

    expect(await screen.findByText(/Could not save that/)).toBeTruthy();
  });

  it('carries the Adzuna attribution only for Adzuna', () => {
    // Required by Adzuna's API terms, not a courtesy credit.
    const { container } = render(JobSourceRow, { ...props, source: 'adzuna' });
    expect(container.textContent).toContain('Adzuna');

    const other = render(JobSourceRow, props);
    expect(other.container.textContent).not.toContain('jobs by');
  });
});
