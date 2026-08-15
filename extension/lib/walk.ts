/**
 * The order an autofill works through the form, as a value.
 *
 * The panel fills one question at a time so the user can watch it happen, which
 * means the sequence has state: where it is, what it applied, what it had to
 * skip, and whether the user stopped it. Keeping that state here — pure, with no
 * timers and no messaging — is what makes it testable; App.svelte supplies the
 * pauses and the wire.
 */

export interface Walk {
  /** The labels to work through, in order. */
  labels: string[];
  /** How far along the list the walk has got. */
  at: number;
  /** Labels whose value was written. */
  done: string[];
  /** Labels the page no longer carried when their turn came. */
  skipped: string[];
  stopped: boolean;
}

/** A walk over these labels, at the start. */
export function startWalk(labels: string[]): Walk {
  return { labels, at: 0, done: [], skipped: [], stopped: false };
}

/** The label to work on now, or null when the walk is over or stopped. */
export function nextStep(walk: Walk): string | null {
  if (walk.stopped || walk.at >= walk.labels.length) return null;
  return walk.labels[walk.at] ?? null;
}

/** Records that `label` was written, and moves on. */
export function applyStep(walk: Walk, label: string): Walk {
  return { ...walk, at: walk.at + 1, done: [...walk.done, label] };
}

/**
 * Records that `label` was not there to write, and moves on. A question that
 * vanished mid-walk (the form re-rendered, a step collapsed) does not end the
 * walk: the questions after it are still worth answering.
 */
export function skipStep(walk: Walk, label: string): Walk {
  return { ...walk, at: walk.at + 1, skipped: [...walk.skipped, label] };
}

/** Ends the walk. What it already wrote stays on the page — this stops, it does
 *  not undo. */
export function stopWalk(walk: Walk): Walk {
  return { ...walk, stopped: true };
}
