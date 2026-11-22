import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const saveJob = vi.fn();
const dismissJob = vi.fn();
const unsaveJob = vi.fn();
const undismissJob = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: {
    meFeed: (...a: unknown[]) => meFeed(...a),
    saveJob: (...a: unknown[]) => saveJob(...a),
    dismissJob: (...a: unknown[]) => dismissJob(...a),
    unsaveJob: (...a: unknown[]) => unsaveJob(...a),
    undismissJob: (...a: unknown[]) => undismissJob(...a),
    plans: vi.fn(),
  },
}));
const announce = vi.fn();
vi.mock('#lib/announce.svelte.js', () => ({ announce: (...a: unknown[]) => announce(...a) }));
vi.mock('./JobMatch.svelte', async () => {
  const stub = await import('../../../vitest-stubs/JobMatchMarker.svelte');
  return { default: stub.default };
});
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
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
});
