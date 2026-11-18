import { api } from '#lib/api.js';
import { nextResumePollDelayMs, type CvParseState } from '#lib/onboardingResumeWait.js';

/** The bank's reading of the server's `parse_status`. */
function parseState(status: string | undefined): CvParseState {
  return status === 'failed' ? 'failed' : 'idle';
}

/**
 * Whether the candidate's résumé failed to parse — the experience bank's own answer to the
 * question an empty-looking bank raises, surfaced there (and in Tailor's Experience tab,
 * which mounts the same component) rather than only once, during onboarding.
 */
export class ResumeParseStatus {
  state = $state<CvParseState>('idle');
  // Supersedes an in-flight wait the same way onboarding's own `waitToken` does: a newer
  // retry, or the component going away, must stop a stale poll from overwriting a fresher
  // state.
  #waitToken = 0;

  /** `onParsed` reloads the bank once a retried extraction lands. */
  constructor(private readonly onParsed: () => Promise<void>) {}

  /** The component went away: no poll may write after this. */
  stop = () => {
    this.#waitToken++;
  };

  async load() {
    try {
      const resume = await api.getResume();
      if (!resume.enabled || !resume.present) {
        this.state = 'idle';
        return;
      }
      this.state = parseState(resume.parse_status);
    } catch {
      // A failed status read is not evidence of a failed parse — leave whatever state was
      // already shown rather than flashing a banner that was never confirmed.
    }
  }

  /** Re-derives the structured résumé from what is already stored (no re-upload), then polls
   *  the same way onboarding's `waitForResumeStructure` does — the backoff table it uses
   *  (`nextResumePollDelayMs`) is shared; what happens with each state is not, since this
   *  reloads the bank instead of filling wizard fields. */
  async retry() {
    const token = ++this.#waitToken;
    this.state = 'waiting';
    try {
      await api.retryResumeExtract();
    } catch {
      if (token !== this.#waitToken) return;
      this.state = 'failed';
      return;
    }
    for (let attempt = 0; ; attempt++) {
      if (token !== this.#waitToken) return;
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) {
        // Giving up is not the same as failing (see onboardingResumeWait.ts's own doc
        // comment): the retry may still land after we stop asking. Showing 'failed' here
        // would claim an outcome we never actually confirmed; the next time the bank loads,
        // load() reads whatever the server eventually settled on.
        this.state = 'idle';
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, delay));
      if (token !== this.#waitToken) return;
      let status: string | undefined;
      try {
        const resume = await api.getResume();
        status = resume.parse_status;
      } catch {
        continue; // treated as still pending, same as onboarding's own read failure
      }
      if (token !== this.#waitToken) return;
      if (status === 'pending') continue;
      if (status === 'failed') {
        this.state = 'failed';
        return;
      }
      this.state = 'idle';
      await this.onParsed();
      return;
    }
  }
}
