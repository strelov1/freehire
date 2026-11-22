// How far an unattended tailoring run has got, read off the tailored CV the run rewrites.
//
// The run itself starts in the freehire tab (the server lets only a signed-in browser start
// one, so the candidate can watch and undo it). Before its first step the server writes every
// requirement of the vacancy onto the CV as `not_reached`, and each requirement the run gets
// to is re-stamped with what it made of it. Counting the stamped ones is the progress.

/** One requirement of the run's report, as `GET /me/cvs/:id` returns it. */
interface AutopilotEntry {
  requirement: string;
  status: string;
}

export interface TailorProgress {
  /** Requirements the run has reached, whatever it made of them. */
  done: number;
  /** Requirements the run set out to cover; 0 until it has laid down its plan. */
  total: number;
}

export function tailorProgress(report: unknown): TailorProgress {
  const entries = Array.isArray(report) ? (report as AutopilotEntry[]) : [];
  return { total: entries.length, done: entries.filter((e) => e?.status && e.status !== 'not_reached').length };
}

/**
 * How long a run that has started may leave the CV untouched before it is taken as over.
 * The server sends no "finished": a run that stops at its step cap leaves requirements
 * `not_reached` for good, so silence is the only end it announces.
 */
export const TAILOR_IDLE_MS = 120_000;

/** Whether the run is over: every requirement reached, or the CV quiet for `TAILOR_IDLE_MS`. */
export function tailorFinished(p: TailorProgress, idleMs: number): boolean {
  if (p.total === 0) return false;
  return p.done >= p.total || idleMs >= TAILOR_IDLE_MS;
}
