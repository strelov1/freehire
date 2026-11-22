import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const markFeedSeen = vi.fn();
const saveJob = vi.fn();
const dismissJob = vi.fn();
const unsaveJob = vi.fn();
const undismissJob = vi.fn();
const markJobApplied = vi.fn();
const untrackApplication = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: {
    meFeed: (...a: unknown[]) => meFeed(...a),
    markFeedSeen: (...a: unknown[]) => markFeedSeen(...a),
    saveJob: (...a: unknown[]) => saveJob(...a),
    dismissJob: (...a: unknown[]) => dismissJob(...a),
    unsaveJob: (...a: unknown[]) => unsaveJob(...a),
    undismissJob: (...a: unknown[]) => undismissJob(...a),
    markJobApplied: (...a: unknown[]) => markJobApplied(...a),
    untrackApplication: (...a: unknown[]) => untrackApplication(...a),
    plans: vi.fn(),
  },
}));
vi.mock('#lib/analytics.js', () => ({ track: vi.fn() }));
const announce = vi.fn();
vi.mock('#lib/announce.svelte.js', () => ({ announce: (...a: unknown[]) => announce(...a) }));
vi.mock('./JobMatch.svelte', async () => {
  const stub = await import('../../../vitest-stubs/JobMatchMarker.svelte');
  return { default: stub.default };
});
vi.mock('$app/paths', () => ({
  resolve: (path: string, p?: Record<string, string>) => path.replace(/\[(\w+)\]/g, (_, k) => p?.[k] ?? ''),
}));
vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/matches') } }));

const { default: MatchSwipe } = await import('./MatchSwipe.svelte');

describe('MatchSwipe', () => {
  it('shows the upgrade screen, not an error, when the plan lapsed (402)', async () => {
    meFeed.mockRejectedValue(new ApiError(402, 'The match feed needs Pro or Ultra'));
    render(MatchSwipe, { onClose: () => {} });
    expect(await screen.findByText('Part of Pro and Ultra')).toBeTruthy();
    expect(screen.queryByText('Failed to load your matches.')).toBeNull();
  });

  it('puts the match block on the card in place of JobMatch, and Save still saves', async () => {
    const card = {
      public_slug: 'a-job',
      title: 'A Job',
      company: 'Acme',
      skills: [] as string[],
      match: { why: ['Go'], may_not_fit: [], unknown: [] },
    };
    meFeed.mockResolvedValue({ data: [card, { ...card, public_slug: 'b-job', title: 'B Job' }] });
    saveJob.mockResolvedValue(undefined);
    render(MatchSwipe, { onClose: () => {} });
    expect(await screen.findByText('Fits')).toBeTruthy();
    expect(screen.queryByText('JOBMATCH-MARKER')).toBeNull();

    await fireEvent.click(screen.getByLabelText('Save'));
    expect(saveJob).toHaveBeenCalledWith('a-job');
    await vi.waitFor(() => expect(screen.getByLabelText('Job: B Job at Acme')).toBeTruthy());
  });

  it('saves a card dragged past the commit distance', async () => {
    const card = {
      public_slug: 'drag-job',
      title: 'Drag Job',
      company: 'Acme',
      skills: [] as string[],
      match: { why: ['Go'], may_not_fit: [], unknown: [] },
    };
    meFeed.mockResolvedValue({ data: [card] });
    saveJob.mockResolvedValue(undefined);
    render(MatchSwipe, { onClose: () => {} });
    const group = await screen.findByLabelText('Job: Drag Job at Acme');
    group.setPointerCapture = vi.fn();
    const ev = (type: string, clientX: number) =>
      Object.assign(new MouseEvent(type, { bubbles: true, clientX, clientY: 0 }), { pointerId: 1 });
    await fireEvent(group, ev('pointerdown', 0));
    await fireEvent(group, ev('pointermove', 120));
    await fireEvent(group, ev('pointerup', 120));
    expect(saveJob).toHaveBeenCalledWith('drag-job');
  });

  describe('actions and keys', () => {
    const mk = (slug: string, title: string) => ({
      public_slug: slug,
      title,
      company: 'Acme',
      skills: [] as string[],
      match: { why: ['Go'], may_not_fit: [], unknown: [] },
    });
    async function open() {
      meFeed.mockResolvedValue({ data: [mk('a-job', 'A Job'), mk('b-job', 'B Job')] });
      saveJob.mockResolvedValue(undefined);
      dismissJob.mockResolvedValue(undefined);
      unsaveJob.mockResolvedValue(undefined);
      render(MatchSwipe, { onClose: () => {} });
      await screen.findByLabelText('Job: A Job at Acme');
      return screen.getByRole('dialog');
    }
    const shown = (t: string) => vi.waitFor(() => expect(screen.getByLabelText(`Job: ${t} at Acme`)).toBeTruthy());
    const undoBtn = () => screen.getByLabelText('Undo last') as HTMLButtonElement;
    beforeEach(() => {
      announce.mockClear();
      saveJob.mockReset();
      dismissJob.mockReset();
      unsaveJob.mockReset();
    });

    it('ArrowRight saves and announces the next card', async () => {
      const dlg = await open();
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      expect(saveJob).toHaveBeenCalledWith('a-job');
      await shown('B Job');
      expect(announce).toHaveBeenCalledWith('Saved. Next: B Job at Acme');
    });

    it('a failed skip puts the card back with a toast and an assertive announce', async () => {
      const dlg = await open();
      dismissJob.mockRejectedValue(new ApiError(500, 'boom'));
      await fireEvent.keyDown(dlg, { key: 'ArrowLeft' });
      expect(await screen.findByText("Didn't skip — try again")).toBeTruthy();
      await shown('A Job');
      expect(announce).toHaveBeenCalledWith("Didn't skip — try again", 'assertive');
    });

    it('a 404 on save drops the card without a toast and disables Undo', async () => {
      const dlg = await open();
      saveJob.mockRejectedValue(new ApiError(404, 'gone'));
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await shown('B Job');
      await vi.waitFor(() => expect(undoBtn().disabled).toBe(true));
      expect(screen.queryByText(/try again/)).toBeNull();
    });

    it('a failed undo takes the card away again, keeps Undo and toasts', async () => {
      const dlg = await open();
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await shown('B Job');
      unsaveJob.mockRejectedValue(new ApiError(500, 'boom'));
      await fireEvent.keyDown(dlg, { key: 'u' });
      expect(await screen.findByText("Didn't undo — try again")).toBeTruthy();
      await shown('B Job');
      expect(undoBtn().disabled).toBe(false);
    });

    it('undo announces the card is back', async () => {
      const dlg = await open();
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await shown('B Job');
      await fireEvent.keyDown(dlg, { key: 'U' });
      await shown('A Job');
      await vi.waitFor(() => expect(announce).toHaveBeenCalledWith('Undone. A Job is back.'));
    });

    it('undo waits for the save to land before unsaving', async () => {
      const dlg = await open();
      let landed!: () => void;
      saveJob.mockReturnValue(new Promise<void>((r) => (landed = r)));
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await shown('B Job');
      await fireEvent.keyDown(dlg, { key: 'u' });
      await shown('A Job');
      expect(unsaveJob).not.toHaveBeenCalled();
      landed();
      await vi.waitFor(() => expect(unsaveJob).toHaveBeenCalledWith('a-job'));
    });

    it('ignores a new judgment while an Undo waits for the save', async () => {
      const dlg = await open();
      let landed!: () => void;
      saveJob.mockReturnValueOnce(new Promise<void>((r) => (landed = r)));
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await shown('B Job');
      await fireEvent.keyDown(dlg, { key: 'u' });
      await shown('A Job');
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      expect(saveJob).toHaveBeenCalledTimes(1);
      landed();
      await vi.waitFor(() => expect(unsaveJob).toHaveBeenCalledWith('a-job'));
    });

    it('offers Undo on the caught-up screen after the last card', async () => {
      meFeed.mockResolvedValueOnce({ data: [mk('a-job', 'A Job')] }).mockResolvedValue({ data: [] });
      saveJob.mockResolvedValue(undefined);
      unsaveJob.mockResolvedValue(undefined);
      render(MatchSwipe, { onClose: () => {} });
      await fireEvent.click(await screen.findByLabelText('Save'));
      await fireEvent.click(await screen.findByText('Undo last'));
      await shown('A Job');
      await vi.waitFor(() => expect(unsaveJob).toHaveBeenCalledWith('a-job'));
    });

    it('ignores modifiers, repeats and fields', async () => {
      const dlg = await open();
      await fireEvent.keyDown(dlg, { key: 'ArrowRight', ctrlKey: true });
      await fireEvent.keyDown(dlg, { key: 'ArrowRight', repeat: true });
      const input = document.createElement('input');
      dlg.appendChild(input);
      await fireEvent.keyDown(input, { key: 'ArrowRight' });
      const item = document.createElement('div');
      item.setAttribute('role', 'menuitem');
      dlg.appendChild(item);
      await fireEvent.keyDown(item, { key: 'ArrowRight' });
      expect(saveJob).not.toHaveBeenCalled();
    });
  });

  describe('apply', () => {
    const mk = (slug: string, title: string, extra: Record<string, unknown> = {}) => ({
      public_slug: slug,
      title,
      company: 'Acme',
      url: `https://acme.test/${slug}`,
      skills: [] as string[],
      match: { why: ['Go'], may_not_fit: [], unknown: [] },
      ...extra,
    });
    async function open(first: Record<string, unknown> = {}) {
      meFeed.mockResolvedValue({ data: [mk('a-job', 'A Job', first), mk('b-job', 'B Job')] });
      markJobApplied.mockResolvedValue({ application_id: 'a17' });
      untrackApplication.mockResolvedValue({});
      render(MatchSwipe, { onClose: () => {} });
      await screen.findByLabelText('Job: A Job at Acme');
      return screen.getByRole('dialog');
    }
    const shown = (t: string) => vi.waitFor(() => expect(screen.getByLabelText(`Job: ${t} at Acme`)).toBeTruthy());
    const applyLink = () => screen.getByLabelText("Apply to A Job on the employer's site (opens in a new tab)");
    const alreadyApplied = async () => {
      await fireEvent.click(screen.getByLabelText('More actions for Acme'));
      await fireEvent.click(await screen.findByRole('menuitem', { name: 'Already applied' }));
    };
    beforeEach(() => {
      announce.mockClear();
      markJobApplied.mockReset();
      untrackApplication.mockReset();
      saveJob.mockReset();
      dismissJob.mockReset();
    });

    it('external Apply opens the employer, marks applied, advances and offers Undo in the toast', async () => {
      await open();
      const link = applyLink() as HTMLAnchorElement;
      expect(link.getAttribute('href')).toBe('https://acme.test/a-job');
      expect(link.getAttribute('target')).toBe('_blank');
      await fireEvent.click(link);
      expect(markJobApplied).toHaveBeenCalledWith('a-job');
      await shown('B Job');
      expect(await screen.findByText('Applied to A Job, opened in a new tab.')).toBeTruthy();
      expect(announce).toHaveBeenCalledWith('Applied to A Job, opened in a new tab. Next: B Job at Acme');
    });

    it('Undo from the toast deletes only the application by its id', async () => {
      await open();
      await fireEvent.click(applyLink());
      await shown('B Job');
      await fireEvent.click(await screen.findByRole('button', { name: 'Undo' }));
      await shown('A Job');
      expect(untrackApplication).toHaveBeenCalledWith('a17');
      expect(unsaveJob).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(announce).toHaveBeenCalledWith('Undone. A Job is back.'));
    });

    it('Undo is ignored while the applied card is still flying off', async () => {
      const dlg = await open();
      await fireEvent.click(applyLink());
      await fireEvent.keyDown(dlg, { key: 'u' });
      await shown('B Job');
      expect(screen.queryByLabelText('Job: A Job at Acme')).toBeNull();
      expect(untrackApplication).not.toHaveBeenCalled();
    });

    it('drops the toast Undo once another card is judged', async () => {
      await open();
      await fireEvent.click(applyLink());
      await shown('B Job');
      await screen.findByRole('button', { name: 'Undo' });
      dismissJob.mockResolvedValue(undefined);
      await fireEvent.click(screen.getByLabelText('Skip'));
      await vi.waitFor(() => expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull());
    });

    it('offers no Undo when the application already existed (no application_id)', async () => {
      await open();
      markJobApplied.mockResolvedValue({});
      await fireEvent.click(applyLink());
      await shown('B Job');
      expect(await screen.findByText('Applied to A Job, opened in a new tab.')).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();
      await vi.waitFor(() => expect((screen.getByLabelText('Undo last') as HTMLButtonElement).disabled).toBe(true));
    });

    it('Already applied marks, advances and announces, without a toast or a tab', async () => {
      await open();
      await alreadyApplied();
      expect(markJobApplied).toHaveBeenCalledWith('a-job');
      await shown('B Job');
      await vi.waitFor(() => expect(announce).toHaveBeenCalledWith('Marked as applied. Next: B Job at Acme'));
      expect(screen.queryByText(/opened in a new tab/)).toBeNull();
    });

    it('U undoes an Already applied by application id', async () => {
      const dlg = await open();
      await alreadyApplied();
      await shown('B Job');
      await fireEvent.keyDown(dlg, { key: 'u' });
      await shown('A Job');
      expect(untrackApplication).toHaveBeenCalledWith('a17');
    });

    it('direct apply links to the job page and writes nothing', async () => {
      await open({ direct_apply: true });
      const link = screen.getByLabelText('Apply to A Job on freehire (opens in a new tab)') as HTMLAnchorElement;
      expect(link.getAttribute('href')).toBe('/jobs/a-job');
      await fireEvent.click(link);
      expect(markJobApplied).not.toHaveBeenCalled();
      expect(screen.getByLabelText('Job: A Job at Acme')).toBeTruthy();
    });

    it('a closed job has no Apply but keeps the menu', async () => {
      await open({ closed_at: '2026-11-01T00:00:00Z' });
      expect(screen.queryByLabelText(/^Apply to /)).toBeNull();
      expect(screen.getByLabelText('More actions for Acme')).toBeTruthy();
    });

    it('a failed apply brings the card back with a toast and an assertive announce', async () => {
      await open();
      markJobApplied.mockRejectedValue(new ApiError(500, 'boom'));
      await fireEvent.click(applyLink());
      expect(await screen.findByText("Didn't mark as applied — try again")).toBeTruthy();
      await shown('A Job');
      expect(announce).toHaveBeenCalledWith("Didn't mark as applied — try again", 'assertive');
    });

    it('a 404 on apply drops the card silently and disables Undo', async () => {
      await open();
      markJobApplied.mockRejectedValue(new ApiError(404, 'gone'));
      await alreadyApplied();
      await shown('B Job');
      await vi.waitFor(() => expect((screen.getByLabelText('Undo last') as HTMLButtonElement).disabled).toBe(true));
      expect(screen.queryByText(/try again/)).toBeNull();
    });

    it('arrow keys judge nothing while the menu is open', async () => {
      const dlg = await open();
      await fireEvent.click(screen.getByLabelText('More actions for Acme'));
      await screen.findByRole('menu');
      await fireEvent.keyDown(dlg, { key: 'ArrowRight' });
      await fireEvent.keyDown(dlg, { key: 'ArrowLeft' });
      expect(saveJob).not.toHaveBeenCalled();
      expect(dismissJob).not.toHaveBeenCalled();
    });
  });

  describe('new first', () => {
    const job = (slug: string, is_new: boolean) => ({
      public_slug: slug,
      title: slug.toUpperCase(),
      company: 'Acme',
      skills: [] as string[],
      is_new,
      match: { why: [], may_not_fit: [], unknown: [] },
    });

    it('badges new cards, pauses before the first rest card and posts seen once', async () => {
      markFeedSeen.mockReset();
      markFeedSeen.mockResolvedValue(undefined);
      meFeed.mockResolvedValue({ data: [job('n1', true), job('r1', false)], meta: { seen_cursor: 's1' } });
      dismissJob.mockResolvedValue(undefined);
      render(MatchSwipe, { onClose: () => {} });
      expect(await screen.findByText('New')).toBeTruthy();
      expect(markFeedSeen).not.toHaveBeenCalled();

      await fireEvent.click(screen.getByLabelText('Skip'));
      expect(await screen.findByText("That's everything new")).toBeTruthy();
      expect(screen.queryByText('R1')).toBeNull();
      expect(markFeedSeen).toHaveBeenCalledTimes(1);
      expect(markFeedSeen).toHaveBeenCalledWith('s1');

      await fireEvent.click(screen.getByText('Show the rest'));
      expect(await screen.findByText('R1')).toBeTruthy();
      expect(screen.queryByText('New')).toBeNull();
      expect(markFeedSeen).toHaveBeenCalledTimes(1);
    });

    it('ignores the card keys and hides the menu while the pause screen is up', async () => {
      meFeed.mockResolvedValue({ data: [job('r1', false)], meta: { seen_cursor: 's1' } });
      markFeedSeen.mockResolvedValue(undefined);
      saveJob.mockClear();
      dismissJob.mockClear();
      meFeed.mockResolvedValue({ data: [job('n1', true), job('r1', false)], meta: { seen_cursor: 's1' } });
      dismissJob.mockResolvedValue(undefined);
      render(MatchSwipe, { onClose: () => {} });
      await fireEvent.click(await screen.findByLabelText('Skip'));
      await screen.findByText("That's everything new");
      dismissJob.mockClear();
      await fireEvent.keyDown(screen.getByRole('dialog'), { key: 'ArrowLeft' });
      await fireEvent.keyDown(screen.getByRole('dialog'), { key: 'ArrowRight' });
      expect(dismissJob).not.toHaveBeenCalled();
      expect(saveJob).not.toHaveBeenCalled();
      expect(screen.queryByLabelText(/More actions for/)).toBeNull();
    });

    it('goes straight to the cards when nothing is new', async () => {
      markFeedSeen.mockReset();
      meFeed.mockResolvedValue({ data: [job('r1', false)], meta: {} });
      render(MatchSwipe, { onClose: () => {} });
      expect(await screen.findByText('R1')).toBeTruthy();
      expect(screen.queryByText("That's everything new")).toBeNull();
      expect(markFeedSeen).not.toHaveBeenCalled();
    });
  });
});
