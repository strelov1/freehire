import { describe, it, expect } from 'vitest';
import { startWalk, nextStep, applyStep, skipStep, stopWalk } from './walk';

describe('walk', () => {
  it('offers the labels in the order it was given them', () => {
    let walk = startWalk(['First name', 'Email', 'City']);

    const seen: string[] = [];
    for (;;) {
      const step = nextStep(walk);
      if (!step) break;
      seen.push(step);
      walk = applyStep(walk, step);
    }

    expect(seen).toEqual(['First name', 'Email', 'City']);
  });

  it('is done once every label has been applied', () => {
    let walk = startWalk(['Email']);
    walk = applyStep(walk, 'Email');

    expect(nextStep(walk)).toBeNull();
    expect(walk.done).toEqual(['Email']);
  });

  // The user pressed Stop. What is already on the page stays there — this ends the
  // walk, it does not undo it.
  it('offers nothing more once stopped, and keeps what it applied', () => {
    let walk = startWalk(['First name', 'Email', 'City']);
    walk = applyStep(walk, 'First name');
    walk = applyStep(walk, 'Email');
    walk = stopWalk(walk);

    expect(nextStep(walk)).toBeNull();
    expect(walk.done).toEqual(['First name', 'Email']);
    expect(walk.stopped).toBe(true);
  });

  // A form re-renders mid-walk and drops a question. Skipping it is not the end of
  // the walk — the questions after it are still there to answer.
  it('records a skip and carries on', () => {
    let walk = startWalk(['First name', 'Gone', 'City']);
    walk = applyStep(walk, 'First name');
    walk = skipStep(walk, 'Gone');

    expect(nextStep(walk)).toBe('City');
    walk = applyStep(walk, 'City');
    expect(walk.done).toEqual(['First name', 'City']);
    expect(walk.skipped).toEqual(['Gone']);
  });

  it('has nothing to offer for an empty plan', () => {
    const walk = startWalk([]);

    expect(nextStep(walk)).toBeNull();
    expect(walk.done).toEqual([]);
  });
});
