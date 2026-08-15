/**
 * The order an autofill works through the form, as a value.
 *
 * The panel fills one question at a time so the user can watch it happen, which
 * means the sequence has state: where it is, what it applied, what it had to
 * skip, and whether the user stopped it. Keeping that state here — pure, with no
 * timers and no messaging — is what makes it testable; App.svelte supplies the
 * pauses and the wire.
 */

export interface Walk<T> {
  /** The steps to work through, in order. */
  steps: T[];
  /** How far along the list the walk has got. */
  at: number;
  /** Steps whose value was written. */
  done: T[];
  /** Steps the page no longer carried when their turn came. */
  skipped: T[];
  stopped: boolean;
}

/**
 * A walk over these steps, at the start. Generic in the step so the caller keeps
 * whatever it needs to act on — a fill carries the value and the frame it
 * belongs to, the agent's report carries a bare label — instead of the walk
 * holding labels and the caller indexing back into its own list to find the rest.
 */
export function startWalk<T>(steps: T[]): Walk<T> {
  return { steps, at: 0, done: [], skipped: [], stopped: false };
}

/** The step to work on now, or null when the walk is over or stopped. */
export function nextStep<T>(walk: Walk<T>): T | null {
  if (walk.stopped || walk.at >= walk.steps.length) return null;
  return walk.steps[walk.at] ?? null;
}

/** Records that the step was written, and moves on. */
export function applyStep<T>(walk: Walk<T>, step: T): Walk<T> {
  return { ...walk, at: walk.at + 1, done: [...walk.done, step] };
}

/**
 * Records that the step was not there to write, and moves on. A question that
 * vanished mid-walk (the form re-rendered, a step collapsed) does not end the
 * walk: the questions after it are still worth answering.
 */
export function skipStep<T>(walk: Walk<T>, step: T): Walk<T> {
  return { ...walk, at: walk.at + 1, skipped: [...walk.skipped, step] };
}

/** Ends the walk. What it already wrote stays on the page — this stops, it does
 *  not undo. */
export function stopWalk<T>(walk: Walk<T>): Walk<T> {
  return { ...walk, stopped: true };
}
