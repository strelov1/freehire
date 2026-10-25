import { tick } from 'svelte';
import { errorMessage } from '#lib/utils.js';

/**
 * A failed MUTATION over a view the user is in the middle of: shown as an inline Alert, so
 * the list (and any editor still open in it) stays as it was. A failed LOAD leaves nothing
 * to show and takes the view through `States` instead.
 */
export class ActionError {
  message = $state('');

  /**
   * The Alert that shows it carries role="alert", which already announces it; clearing it
   * first re-inserts the text, so the same failure twice in a row is announced again.
   */
  fail(e: unknown, fallback: string): void {
    const message = errorMessage(e, fallback);
    this.message = '';
    void tick().then(() => (this.message = message));
  }

  clear(): void {
    this.message = '';
  }
}
