import { browser } from 'wxt/browser';

/**
 * Where the hire session lives. Set `WXT_HIRE_ORIGIN` to build against a
 * deployment (`.env.production` points at freehire.me); the default is the local
 * SPA origin, not the API port, so the launchWebAuthFlow popup carries the
 * session cookie and the /api proxy forwards it — mirroring production, where
 * SPA and API are one origin.
 */
export const HIRE_ORIGIN = import.meta.env.WXT_HIRE_ORIGIN ?? 'http://localhost:5173';

const TOKEN_KEY = 'hireToken';

/** The signed-in user as /auth/me reports it (only what the panel shows). */
export interface HireUser {
  id: number;
  email: string;
}

/**
 * Extracts the minted token from a launchWebAuthFlow redirect URL, verifying the
 * anti-forgery state. Pure over its inputs. Throws on a state mismatch, an error
 * indication, or a missing token.
 */
export function parseAuthRedirect(responseUrl: string, expectedState: string): string {
  const params = new URLSearchParams(new URL(responseUrl).hash.slice(1));
  if (params.get('state') !== expectedState) {
    throw new Error('auth state mismatch');
  }
  const error = params.get('error');
  if (error) {
    throw new Error(`sign-in failed: ${error}`);
  }
  const token = params.get('token');
  if (!token) {
    throw new Error('sign-in returned no token');
  }
  return token;
}

/** Runs the "Sign in with freehire" flow and stores the resulting token. */
export async function signIn(): Promise<string> {
  const redirectUri = browser.identity.getRedirectURL();
  const state = crypto.randomUUID();
  const authUrl =
    `${HIRE_ORIGIN}/api/v1/auth/extension/connect` +
    `?redirect_uri=${encodeURIComponent(redirectUri)}&state=${encodeURIComponent(state)}`;

  const responseUrl = await browser.identity.launchWebAuthFlow({ url: authUrl, interactive: true });
  if (!responseUrl) {
    throw new Error('sign-in was cancelled');
  }
  const token = parseAuthRedirect(responseUrl, state);
  await browser.storage.local.set({ [TOKEN_KEY]: token });
  return token;
}

/** Clears the stored token. */
export async function signOut(): Promise<void> {
  await browser.storage.local.remove(TOKEN_KEY);
}

/** The stored token, or null if not signed in. */
export async function getToken(): Promise<string | null> {
  const stored = await browser.storage.local.get(TOKEN_KEY);
  return (stored[TOKEN_KEY] as string) ?? null;
}

/**
 * Resolves the signed-in user for a token, or null if the token is missing or no
 * longer valid (e.g. revoked). The extension's host_permissions exempt this
 * cross-origin Bearer call from CORS.
 */
export async function fetchMe(token: string): Promise<HireUser | null> {
  const res = await fetch(`${HIRE_ORIGIN}/api/v1/auth/me`, {
    credentials: 'omit',
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) return null;
  const body = (await res.json()) as { data: HireUser };
  return body.data;
}
