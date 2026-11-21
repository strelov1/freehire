import { beforeEach, describe, expect, it, vi } from 'vitest';

const motion = vi.hoisted(() => ({ reduced: false }));
vi.mock('svelte/motion', () => ({
  prefersReducedMotion: {
    get current() {
      return motion.reduced;
    },
  },
}));

const { AXIS_LOCK_PX, COMMIT_PX, SwipeGesture } = await import('./swipeGesture.svelte.js');

const capture = vi.fn();
const pt = (x: number, y: number, over: Record<string, unknown> = {}) => ({
  clientX: x,
  clientY: y,
  pointerId: 1,
  target: { closest: () => null },
  currentTarget: { setPointerCapture: capture },
  ...over,
});

function drag(g: InstanceType<typeof SwipeGesture>, dx: number, dy = 0) {
  g.down(pt(0, 0));
  g.move(pt(dx, dy));
}

beforeEach(() => {
  motion.reduced = false;
  capture.mockReset();
  window.innerWidth = 400;
});

describe('SwipeGesture', () => {
  it('commits save past the threshold to the right', () => {
    const g = new SwipeGesture();
    const commit = vi.fn();
    drag(g, COMMIT_PX + 20);
    expect(capture).toHaveBeenCalledWith(1);
    g.up(commit);
    expect(commit).toHaveBeenCalledWith('save');
  });

  it('commits dismiss past the threshold to the left', () => {
    const g = new SwipeGesture();
    const commit = vi.fn();
    drag(g, -(COMMIT_PX + 20));
    g.up(commit);
    expect(commit).toHaveBeenCalledWith('dismiss');
  });

  it('springs back on a short drag without committing', () => {
    const g = new SwipeGesture();
    const commit = vi.fn();
    drag(g, 40);
    expect(g.dragX).toBe(40);
    g.up(commit);
    expect(commit).not.toHaveBeenCalled();
    expect(g.dragX).toBe(0);
    expect(g.dragging).toBe(false);
  });

  it('leaves a vertical-first gesture to native scroll', () => {
    const g = new SwipeGesture();
    const commit = vi.fn();
    g.down(pt(0, 0));
    g.move(pt(2, AXIS_LOCK_PX + 12));
    g.move(pt(200, AXIS_LOCK_PX + 12));
    g.up(commit);
    expect(g.dragging).toBe(false);
    expect(capture).not.toHaveBeenCalled();
    expect(commit).not.toHaveBeenCalled();
  });

  it('does not start a drag from a link or button', () => {
    const g = new SwipeGesture();
    g.down(pt(0, 0, { target: { closest: () => ({}) } }));
    g.move(pt(150, 0));
    expect(g.dragging).toBe(false);
    expect(g.dragX).toBe(0);
  });

  it('caps the tilt and fades the stamp in with distance', () => {
    const g = new SwipeGesture();
    drag(g, 50);
    expect(g.stampOpacity).toBe(0.5);
    drag(g, 1000);
    expect(g.rotation).toBe(18);
    expect(g.stampOpacity).toBe(1);
  });

  it('flings off-screen toward its side', () => {
    const g = new SwipeGesture();
    g.fling('dismiss');
    expect(g.exiting).toBe(true);
    expect(g.dragX).toBe(-600);
    expect(g.opacity).toBe(0);
    g.settle();
    expect(g.dragX).toBe(0);
    expect(g.exiting).toBe(false);
  });

  it('only fades under reduced motion: no sweep, no tilt', () => {
    motion.reduced = true;
    const g = new SwipeGesture();
    g.fling('save');
    expect(g.dragX).toBe(0);
    expect(g.rotation).toBe(0);
    expect(g.opacity).toBe(0);
  });
});
