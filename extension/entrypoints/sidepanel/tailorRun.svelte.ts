import { getToken } from '../../lib/auth';
import { getCVRunState, getTailoredCVForJob, type TailoredCV } from '../../lib/freehire';
import { tailorFinished, tailorProgress, type TailorProgress } from '../../lib/tailorProgress';

const POLL_MS = 3000;
/** Give up waiting for the freehire tab to create the CV and its run after this long. */
const START_TIMEOUT_MS = 90_000;

// A plain record, not reactive: it is written from inside the card's $derived, where a
// reactive write is refused. The runs' own fields are what the card reacts to.
const runs: Record<string, TailorRun> = {};

/**
 * The run for a job, shared by every card that shows the job: the card is rebuilt whenever
 * the page's match is re-read, and a run that died with it would report the half-made CV
 * as done.
 */
export function tailorRunFor(slug: string): TailorRun {
  return (runs[slug] ??= new TailorRun(slug));
}

/**
 * Follows one tailoring run from the panel. The run runs in the freehire tab the panel
 * opened; this only reads the tailored CV it rewrites, every few seconds, until the run is
 * over (see lib/tailorProgress.ts). `cv` is set once the CV exists, which is what the
 * "Watch it work" link needs; `finished` says the attach may be offered.
 */
export class TailorRun {
  running = $state(false);
  progress = $state<TailorProgress>({ done: 0, total: 0 });
  cv = $state<TailoredCV | null>(null);
  finished = $state(false);
  error = $state('');
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly slug: string) {}

  start() {
    this.stop();
    this.running = true;
    this.finished = false;
    this.error = '';
    this.progress = { done: 0, total: 0 };
    const startedAt = Date.now();
    let lastUpdate = '';
    let lastChangeAt = 0;

    const tick = async () => {
      try {
        const token = await getToken();
        if (!token) throw new Error('signed out');
        this.cv ??= await getTailoredCVForJob(this.slug, token);
        if (!this.cv) {
          if (Date.now() - startedAt > START_TIMEOUT_MS) throw new Error('the freehire tab did not start tailoring');
        } else {
          const state = await getCVRunState(this.cv.id, token);
          if (state.updated_at !== lastUpdate) {
            // The first read dates the quiet from the server's own timestamp, so a CV
            // finished an hour ago reads as over at once rather than after another wait.
            const changed = lastUpdate === '' ? Date.parse(state.updated_at) : NaN;
            lastUpdate = state.updated_at;
            lastChangeAt = Number.isFinite(changed) ? Math.min(changed, Date.now()) : Date.now();
          }
          this.progress = tailorProgress(state.autopilot_report);
          if (tailorFinished(this.progress, Date.now() - lastChangeAt)) {
            this.running = false;
            this.finished = true;
            return;
          }
        }
      } catch (err) {
        this.error = err instanceof Error ? err.message : String(err);
        this.running = false;
        return;
      }
      this.#timer = setTimeout(tick, POLL_MS);
    };
    void tick();
  }

  stop() {
    clearTimeout(this.#timer);
    this.running = false;
  }
}
