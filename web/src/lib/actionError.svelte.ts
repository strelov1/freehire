import { announce } from '#lib/announce.svelte.js';
import { errorMessage } from '#lib/utils.js';

/**
 * A failed MUTATION over a view the user is in the middle of: shown as an inline Alert, so
 * the list (and any editor still open in it) stays as it was. A failed LOAD leaves nothing
 * to show and takes the view through `States` instead.
 *
 * The voice is the shared announcer, not the Alert: a region mounted together with its
 * message is often not read at all (see `announce.svelte.ts`). So the Alert that shows
 * `message` must be rendered with `live={false}`, or the failure is read twice.
 */
export class ActionError {
  message = $state('');

  fail(e: unknown, fallback: string): void {
    this.message = errorMessage(e, fallback);
    announce(this.message, 'assertive');
  }

  clear(): void {
    this.message = '';
  }
}
