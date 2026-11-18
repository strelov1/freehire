import type { Http } from '../api-core';
import type { ConnectedIdentities, User } from '../types';
import { jsonBody } from '../api-core';

export function authApi({ call, requestData }: Http) {
  // --- Auth -----------------------------------------------------------------
  //
  // register/login set the httpOnly auth cookie server-side and return the user;
  // the token never reaches JS. Subsequent calls (me) are authenticated by the
  // cookie the browser attaches automatically.

  /** POST credentials and return the created/authenticated user. */
  async function postAuth(path: string, body: unknown): Promise<User> {
    return requestData<User>(path, jsonBody('POST', body));
  }

  /** Registers with the browser's detected IANA timezone attached, so the account
   *  starts with one set rather than waiting for a profile-page visit. The server
   *  silently drops an unrecognized value rather than failing the signup, so a
   *  detection quirk here is harmless. */
  function register(email: string, password: string): Promise<User> {
    let timezone: string | undefined;
    try {
      timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      timezone = undefined;
    }
    return postAuth('/api/v1/auth/register', { email, password, timezone });
  }

  function login(email: string, password: string): Promise<User> {
    return postAuth('/api/v1/auth/login', { email, password });
  }

  /** Names of OAuth providers enabled on the server (google/github/linkedin).
   *  The dialog renders one "Continue with …" button per name; sign-in itself is
   *  a full-page redirect through /api/v1/auth/oauth/:provider/start. */
  async function oauthProviders(): Promise<string[]> {
    return requestData<string[]>('/api/v1/auth/oauth/providers');
  }

  /** Clear the session cookie server-side. */
  async function logout(): Promise<void> {
    await call('/api/v1/auth/logout', { method: 'POST' });
  }

  /** Fetch the current user using the auth cookie. Throws ApiError(401) if it is
   *  missing or rejected. */
  async function me(): Promise<User> {
    return requestData<User>('/api/v1/auth/me');
  }

  // --- Account recovery -----------------------------------------------------
  //
  // Email verification and password recovery are code-based: the server mails a
  // six-digit code and these endpoints exchange it. The code is never in a URL,
  // so nothing here takes one from the query string.

  /** Ask the server to mail a fresh email-verification code to the signed-in
   *  user's address. 429 while a code issued in the last minute is outstanding. */
  async function requestEmailVerification(): Promise<void> {
    await call('/api/v1/auth/verify/request', { method: 'POST' });
  }

  /** Confirm the signed-in user's address with the mailed code; returns the
   *  updated user (email_verified true). */
  function confirmEmailVerification(code: string): Promise<User> {
    return requestData<User>('/api/v1/auth/verify/confirm', jsonBody('POST', { code }));
  }

  /** Ask for a password-reset code. Always succeeds, whether or not the address
   *  has an account — the response deliberately says nothing either way. */
  async function forgotPassword(email: string): Promise<void> {
    await call('/api/v1/auth/password/forgot', jsonBody('POST', { email }));
  }

  /** Set a new password with a mailed reset code. Every existing session is
   *  revoked, so the caller signs in again afterwards. */
  async function resetPassword(email: string, code: string, password: string): Promise<void> {
    await call('/api/v1/auth/password/reset', jsonBody('POST', { email, code, password }));
  }

  /** Change a known password. Other sessions are revoked; this one is re-issued. */
  async function changePassword(currentPassword: string, password: string): Promise<void> {
    await call('/api/v1/me/password', jsonBody('POST', { current_password: currentPassword, password }));
  }

  /** Mail a code to the address the signed-in user wants to move their account to.
   *  Nothing changes until `confirmEmailChange` spends that code. Gated on recent auth
   *  (428), 409 when another account holds the address, 429 inside the resend cooldown. */
  async function requestEmailChange(email: string): Promise<void> {
    await call('/api/v1/me/email', jsonBody('POST', { email }));
  }

  /** Move the account to the address the code was mailed to; returns the updated user.
   *  Other sessions are revoked; this one is re-issued. */
  function confirmEmailChange(code: string): Promise<User> {
    return requestData<User>('/api/v1/me/email/confirm', jsonBody('POST', { code }));
  }

  async function reauthenticatePassword(password: string): Promise<string> {
    const result = await requestData<{ recent_auth_expires_at: string }>(
      '/api/v2/auth/reauth/password',
      jsonBody('POST', { password }),
    );
    return result.recent_auth_expires_at;
  }

  async function exchangeOAuthReauthentication(code: string, codeVerifier: string): Promise<string> {
    const result = await requestData<{ recent_auth_expires_at: string }>(
      '/api/v2/auth/oauth/exchange',
      jsonBody('POST', { code, code_verifier: codeVerifier }),
    );
    return result.recent_auth_expires_at;
  }

  /** The caller's connected sign-in providers. `identities` is normalized to an array
   *  here because Go marshals an empty slice as `null`, not `[]` — the declared type
   *  says array, so the one place that knows the wire makes that true rather than
   *  every call site defending against it. */
  async function connectedIdentities(): Promise<ConnectedIdentities> {
    const result = await requestData<ConnectedIdentities>('/api/v2/auth/identities');
    return { ...result, identities: result.identities ?? [] };
  }

  /** Sign out everywhere: revokes every session for the account, including this one. */
  async function logoutEverywhere(): Promise<void> {
    await call('/api/v1/auth/logout-all', { method: 'POST' });
  }

  return {
    register,
    login,
    oauthProviders,
    logout,
    me,
    requestEmailVerification,
    confirmEmailVerification,
    forgotPassword,
    resetPassword,
    changePassword,
    requestEmailChange,
    confirmEmailChange,
    reauthenticatePassword,
    exchangeOAuthReauthentication,
    connectedIdentities,
    logoutEverywhere,
  };
}
