import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const meFeed = vi.fn();
const saveJob = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: { meFeed: (...a: unknown[]) => meFeed(...a), saveJob: (...a: unknown[]) => saveJob(...a), plans: vi.fn() },
}));
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
});
