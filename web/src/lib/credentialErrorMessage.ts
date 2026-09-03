import { ApiError } from './api';

/** Shared login/register credential-error copy — an invalid combination (401) and a
 *  malformed request (400: bad email or too-short password) map to the same message
 *  wherever a caller submits email+password. Returns null for anything else, so a
 *  caller with its own additional cases (AuthDialog's password-recovery 429/503,
 *  /onboarding's registration-only 409) checks those first and falls back to this. */
export function credentialErrorMessage(e: unknown): string | null {
  if (!(e instanceof ApiError)) return null;
  if (e.status === 401) return 'Invalid email or password.';
  if (e.status === 400) return 'Enter a valid email and a password of at least 8 characters.';
  return null;
}
