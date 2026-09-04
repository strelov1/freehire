import { ApiError } from './api';
import { promptSignIn } from './signin';

/** Maps a thread/reply submit failure to the message to show under the form. On a
 *  401 (the session lapsed between the gate check and this submit) it sends the
 *  visitor to /signin and returns null (no inline error — there is nothing left to
 *  show once the page is navigating away). This also drops whatever the visitor had
 *  typed: a session lapsing in the ~seconds between opening the box and submitting
 *  is rare enough that persisting the draft across the navigation (the way
 *  SaveSearchAlert's pending-alert handoff persists ITS one piece of intent) is not
 *  worth the extra state for this. Other API errors surface their message; anything
 *  else falls back to a generic message. Shared by the discussion create/reply forms
 *  so they handle failures identically. */
export function communityFormError(err: unknown): string | null {
  if (err instanceof ApiError && err.status === 401) {
    promptSignIn();
    return null;
  }
  if (err instanceof ApiError) {
    return err.message;
  }
  return 'Something went wrong. Please try again.';
}
