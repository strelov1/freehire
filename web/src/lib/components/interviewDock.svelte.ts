import { profileKickoff } from '#lib/assistant/presets.js';

/**
 * The experience interviewer, docked beside the bank. `launch.id` is a remount token, not a
 * session id: aiming the chat at a different set of achievements means a new conversation,
 * and a mounted chat cannot be re-aimed (see the panel).
 */
export class InterviewDock {
  open = $state(false);
  launch = $state({ id: 0, kickoff: profileKickoff([]) });
  // A turn in flight would be abandoned mid-answer by a relaunch, so the entries close
  // while one is running.
  turnActive = $state(false);

  /** Open the interviewer on these achievements (none: the whole bank). */
  launchFor(ids: string[]) {
    if (this.turnActive) return;
    this.launch = { id: this.launch.id + 1, kickoff: profileKickoff(ids) };
    this.open = true;
  }

  close = () => {
    this.open = false;
  };
}
