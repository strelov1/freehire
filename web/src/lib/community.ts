import { ApiError } from './api';
import { openAuthDialog } from './auth-dialog.svelte';

/** Maps a thread/reply submit failure to the message to show under the form. On a
 *  401 it opens the auth dialog and returns null (no inline error); other API errors
 *  surface their message; anything else falls back to a generic message. Shared by
 *  the discussion create/reply forms so they handle failures identically. */
export function communityFormError(err: unknown): string | null {
  if (err instanceof ApiError && err.status === 401) {
    openAuthDialog();
    return null;
  }
  if (err instanceof ApiError) {
    return err.message;
  }
  return 'Something went wrong. Please try again.';
}
