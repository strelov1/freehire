import { api } from '$lib/api';

const verifierKey = 'freehire.reauth.verifier';
const returnKey = 'freehire.reauth.return';

export type ReauthReturnPath = '/my/api-keys' | '/my/security';

function base64url(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
}

export async function beginProviderReauthentication(
  provider: string,
  returnTo: ReauthReturnPath,
): Promise<void> {
  if (!/^[a-z][a-z0-9_-]{1,31}$/.test(provider)) throw new Error('invalid provider');
  const verifierBytes = new Uint8Array(32);
  crypto.getRandomValues(verifierBytes);
  const verifier = base64url(verifierBytes);
  const challenge = base64url(
    new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))),
  );
  sessionStorage.setItem(verifierKey, verifier);
  sessionStorage.setItem(returnKey, returnTo);
  const query = new URLSearchParams({
    platform: 'web',
    code_challenge: challenge,
    code_challenge_method: 'S256',
    callback_target: 'web',
    purpose: 'reauth',
  });
  window.location.assign(`/api/v2/auth/oauth/${encodeURIComponent(provider)}/start?${query}`);
}

export async function completeProviderReauthentication(code: string): Promise<ReauthReturnPath> {
  const verifier = sessionStorage.getItem(verifierKey);
  sessionStorage.removeItem(verifierKey);
  if (!verifier) throw new Error('reauthentication attempt expired');
  await api.exchangeOAuthReauthentication(code, verifier);
  const storedTarget = sessionStorage.getItem(returnKey);
  sessionStorage.removeItem(returnKey);
  return storedTarget === '/my/api-keys' ? storedTarget : '/my/security';
}
