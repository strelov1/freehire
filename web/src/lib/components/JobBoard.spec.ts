import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { MyJob } from '#lib/types.js';
import JobBoard from './JobBoard.svelte';

vi.mock('$app/state', () => ({
  page: { data: { locale: 'en' }, url: new URL('http://localhost/my/tracking'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn(),
}));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true, currentUser: () => null }));
vi.mock('#lib/assistant/api.js', () => ({ createRehearsal: vi.fn(), createDebrief: vi.fn() }));

// EasyMDE stands in as a plain text holder: what is under test is when the board persists
// the notes, not the markdown editor. `typed` is what the next editor will hold.
const editorText = vi.hoisted(() => ({ typed: '' }));
vi.mock('easymde', () => ({
  default: class {
    codemirror = {
      on: () => {},
      getInputField: () => document.createElement('textarea'),
    };
    value() {
      return editorText.typed;
    }
    toTextArea() {}
  },
}));

const { untrackApplication, listMyJobs, trackApplication, saveJob, clearApplicationStage } =
  vi.hoisted(() => ({
    untrackApplication: vi.fn(),
    listMyJobs: vi.fn(),
    trackApplication: vi.fn(),
    saveJob: vi.fn(),
    clearApplicationStage: vi.fn(),
  }));

vi.mock('#lib/api.js', () => ({
  api: { untrackApplication, listMyJobs, trackApplication, saveJob, clearApplicationStage },
  isPlanRefusal: () => false,
}));

// No posting: the panel then fetches nothing of its own, which keeps the test on the board.
const application: MyJob = {
  id: 'app-1',
  company_slug: 'acme',
  role_title: 'Backend Engineer',
  job: null,
  viewed_at: '2026-10-01T00:00:00Z',
  saved_at: null,
  applied_at: '2026-10-01T00:00:00Z',
  stage: 'applied',
  notes: null,
  email_count: 0,
  last_activity_at: null,
  days_silent: null,
  silence_state: null,
  followed_up_at: null,
  cv_opened_at: null,
};

beforeEach(() => {
  untrackApplication.mockReset().mockResolvedValue(undefined);
  listMyJobs.mockReset();
  trackApplication.mockReset();
  editorText.typed = '';
});

describe('JobBoard — empty board', () => {
  it('explains the board and offers a way in instead of empty columns', () => {
    render(JobBoard, { initial: [] });

    expect(screen.getByText('Your board is empty')).toBeTruthy();
    expect(screen.getByText(/Save a job from any listing and it lands here/)).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Find jobs' }).getAttribute('href')).toBe('/jobs');
    expect(screen.getByRole('link', { name: 'Upload your CV' }).getAttribute('href')).toBe(
      '/my/profile',
    );
    // The columns behind it are not drawn at all.
    expect(screen.queryByRole('heading', { name: 'Applied' })).toBeNull();
    expect(screen.queryByRole('searchbox')).toBeNull();
  });

  it('draws the columns, each a named region, once there is a card', () => {
    render(JobBoard, { initial: [application] });

    expect(screen.queryByText('Your board is empty')).toBeNull();
    expect(screen.getByRole('region', { name: 'Applied' })).toBeTruthy();
  });
});

describe('JobBoard — removing an application', () => {
  it('asks before untracking, and does nothing when cancelled', async () => {
    render(JobBoard, { initial: [application], initialId: 'app-1' });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove from board' }));
    expect(screen.getByText('Remove Backend Engineer from your board?')).toBeTruthy();
    expect(untrackApplication).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(untrackApplication).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Remove from board' })).toBeTruthy();
  });

  it('untracks on confirm and drops the card', async () => {
    render(JobBoard, { initial: [application], initialId: 'app-1' });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove from board' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    expect(untrackApplication).toHaveBeenCalledWith('app-1');
    await waitFor(() => expect(screen.getByText('Your board is empty')).toBeTruthy());
  });

  it('keeps the card and says so when the server refuses', async () => {
    untrackApplication.mockRejectedValue(new Error('boom'));
    render(JobBoard, { initial: [application], initialId: 'app-1' });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove from board' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() =>
      expect(screen.getByText("Couldn't remove it from your board. Try again.")).toBeTruthy(),
    );
    expect(screen.queryByText('Your board is empty')).toBeNull();
  });
});

describe('JobBoard — a failed stage change', () => {
  it('rolls the card back and names where it landed', async () => {
    trackApplication.mockRejectedValue(new Error('boom'));
    listMyJobs.mockResolvedValue({ items: [application] });
    render(JobBoard, { initial: [application], view: 'list' });

    const picker = screen.getByRole('combobox', { name: /stage for Backend Engineer/i });
    await fireEvent.change(picker, { target: { value: 'interview' } });

    await waitFor(() =>
      expect(
        screen.getByText("Couldn't move Backend Engineer — it's back in Applied. Try again."),
      ).toBeTruthy(),
    );
  });
});

describe('JobBoard — notes', () => {
  it('saves what was typed when the drawer closes without the editor blurring', async () => {
    trackApplication.mockResolvedValue(undefined);
    render(JobBoard, { initial: [application], initialId: 'app-1' });
    // Let the editor's dynamic import settle before typing into it.
    await waitFor(() => expect(document.querySelector('textarea')).toBeTruthy());
    await new Promise((r) => setTimeout(r, 0));
    editorText.typed = 'Ask about the on-call rota';

    await fireEvent.keyDown(window, { key: 'Escape' });

    await waitFor(() =>
      expect(trackApplication).toHaveBeenCalledWith('app-1', {
        notes: 'Ask about the on-call rota',
      }),
    );
  });
});

describe('JobBoard — the list view drawing a page at a time', () => {
  it('starts from the first page again when the search changes', async () => {
    let reveal: (() => void) | undefined;
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        cb: (e: { isIntersecting: boolean }[]) => void;
        constructor(cb: (e: { isIntersecting: boolean }[]) => void) {
          this.cb = cb;
        }
        observe() {
          reveal = () => this.cb([{ isIntersecting: true }]);
        }
        disconnect() {}
      },
    );
    const rows = Array.from({ length: 120 }, (_, i) => ({ ...application, id: `app-${i}`, role_title: `Role ${i}` }));
    render(JobBoard, { initial: rows, view: 'list' });
    const drawn = () => screen.queryAllByText(/^Role \d+$/).length;

    reveal?.();
    await waitFor(() => expect(drawn()).toBe(100));

    const search = screen.getByRole('searchbox', { name: 'Search applications by company or role' });
    await fireEvent.input(search, { target: { value: 'Role 1' } });
    await fireEvent.input(search, { target: { value: '' } });

    await waitFor(() => expect(drawn()).toBe(50));
    vi.unstubAllGlobals();
  });
});


describe('JobBoard — quick triage', () => {
  const posting = {
    ...application,
    id: 'closed-role',
    saved_at: '2026-10-02T00:00:00Z',
    job: { public_slug: 'closed-role', title: 'Backend Engineer', company: 'Acme', closed_at: '2026-10-30T00:00:00Z' },
  };

  it.each(['board', 'list'] as const)('shows posting closure independently of application stage in %s', (view) => {
    render(JobBoard, { initial: [posting], view });
    expect(screen.getByText('Vacancy closed')).toBeTruthy();
    expect(screen.queryByText('Your board is empty')).toBeNull();
  });

  it.each(['board', 'list'] as const)('removes directly from the %s without opening the drawer', async (view) => {
    render(JobBoard, { initial: [posting], view });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Backend Engineer from board' }));
    expect(untrackApplication).toHaveBeenCalledWith('closed-role');
    await waitFor(() => expect(screen.getByText('Your board is empty')).toBeTruthy());
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('keeps a card and reports a failed quick removal', async () => {
    untrackApplication.mockRejectedValue(new Error('offline'));
    render(JobBoard, { initial: [posting] });
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Backend Engineer from board' }));
    await waitFor(() => expect(screen.getByText("Couldn't remove it from your board. Try again.")).toBeTruthy());
    expect(screen.getByText('Backend Engineer')).toBeTruthy();
  });

  it.each(['board', 'list'] as const)('sorts %s by company and date added', async (view) => {
    const rows = [
      { ...application, id: 'z', role_title: 'Z role', company_slug: 'zeta', added_to_board_at: '2026-10-30T00:00:00Z' },
      { ...application, id: 'a', role_title: 'A role', company_slug: 'alpha', added_to_board_at: '2026-10-01T00:00:00Z' },
    ];
    render(JobBoard, { initial: rows, view });
    const sort = screen.getByRole('combobox', { name: 'Sort applications' });
    await fireEvent.change(sort, { target: { value: 'company' } });
    const titles = () => (view === 'board' ? within(screen.getByRole('region', { name: 'Applied' })) : screen).getAllByText(/^[AZ] role$/).map((el) => el.textContent);
    expect(titles()).toEqual(['A role', 'Z role']);
    await fireEvent.change(sort, { target: { value: 'added' } });
    expect(titles()).toEqual(['Z role', 'A role']);
  });
});
