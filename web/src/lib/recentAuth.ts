import { api } from '$lib/api';

const verifierKey = 'freehire.reauth.verifier';
const returnKey = 'freehire.reauth.return';
const draftKey = 'freehire.reauth.draft';
const expiryKey = 'freehire.reauth.expires';

export type ReauthReturnPath = '/my/api-keys' | '/my/security';

// This module used to be reachable only from a click handler, so it could touch
// `sessionStorage` bare. `recentAuthExpiry()` broke that: it is read at component INIT,
// which runs during SSR, where there is no browser storage at all — and the jsdom test
// project always provides one, so nothing in a component test could ever have caught it.
//
// Reading is also not safe in a browser: Safari with site data blocked THROWS on access
// rather than answering null, which is why `typeof` alone is not the guard. Same shape as
// autoApplyPauseStorage.ts and cvRefreshOffer.ts.
//
// Every read degrades to "nothing carried, no proof held" — the correct reading on a
// server and in a locked-down browser alike. Writes deliberately do NOT degrade: see
// `beginProviderReauthentication`.
function readSession(key: string): string | null {
  try {
    return typeof sessionStorage === 'undefined' ? null : sessionStorage.getItem(key);
  } catch {
    return null;
  }
}

function forgetSession(key: string): void {
  try {
    if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem(key);
  } catch {
    // Nothing to do: the value is unreachable, which is the state we wanted anyway.
  }
}

/** The action a member had begun when confirmation interrupted them. Confirming through
 *  a provider is a full-page navigation, so without carrying this the work is silently
 *  lost — which is the whole complaint behind this transport.
 *
 *  Each surface names itself, so a draft left by one can never be mistaken for another's.
 *  What a surface stores is a deliberate choice, never a dump of its form: no secret and
 *  no anti-mistake barrier belongs here. `delete-account` therefore carries nothing but
 *  its own name — the typed address that arms it is re-entered on return, on purpose. */
export type ReauthDraft =
  | { surface: 'create-api-key'; name: string; days: number }
  | { surface: 'revoke-api-key'; keyId: number }
  | { surface: 'delete-account' };

/** When the proof obtained by the last provider round trip stops being valid, as the SERVER
 *  stated it — both issuing routes answer with `recent_auth_expires_at`, so this is a
 *  measurement and not a lifetime guessed from `RECENT_AUTH_TTL`.
 *
 *  It is reassurance, never permission. The proof can die before its stated expiry (the
 *  session's token version bumped, the proof consumed, the cookie cleared), so a gated call
 *  answering `428` still overrules this — see `forgetRecentAuthExpiry`. An already-passed or
 *  unreadable value reads as no confirmation at all. */
export function recentAuthExpiry(): Date | null {
  const raw = readSession(expiryKey);
  if (!raw) return null;
  const at = Number(raw);
  if (!Number.isFinite(at) || at <= Date.now()) return null;
  return new Date(at);
}

/** Drop the recorded expiry, because the server refused an action this hint said was allowed.
 *  The surface then asks for confirmation again rather than reporting a generic failure. */
export function forgetRecentAuthExpiry(): void {
  forgetSession(expiryKey);
}

/** Whether a parsed value really is the draft its tag claims. `JSON.parse(raw) as
 *  ReauthDraft` is a promise the runtime does not keep, and the restored value goes
 *  straight into an input, so the fields are checked rather than asserted. */
function isDraftFor<S extends ReauthDraft['surface']>(
  value: unknown,
  surface: S,
): value is Extract<ReauthDraft, { surface: S }> {
  if (typeof value !== 'object' || value === null) return false;
  const d = value as Record<string, unknown>;
  if (d.surface !== surface) return false;
  // The caller has already matched the tag; this re-check is what makes the function a
  // real type guard rather than a cast wearing a boolean.
  switch (surface) {
    case 'create-api-key':
      return typeof d.name === 'string' && typeof d.days === 'number';
    case 'revoke-api-key':
      return typeof d.keyId === 'number';
    default:
      // `delete-account` carries nothing but its own name, on purpose: the typed address
      // that arms it is a barrier, not a field to be remembered.
      return true;
  }
}

/** Read the pending action belonging to `surface` and forget it.
 *
 *  Scoped to the asking surface rather than handing over whatever is there: two surfaces
 *  can sit on one route and both consume on mount, and an unscoped read would let the
 *  first to run swallow a draft it does not recognise while the one it belongs to never
 *  sees it. A draft for somebody else is left where it is.
 *
 *  Consumed exactly once, so a draft cannot outlive its trip and reopen a dialog on an
 *  unrelated later visit. A value that is absent, unreadable, or not the shape it claims
 *  is cleared as well — there is nothing to do with it but stop carrying it. */
export function consumeReauthDraft<S extends ReauthDraft['surface']>(
  surface: S,
): Extract<ReauthDraft, { surface: S }> | null {
  const raw = readSession(draftKey);
  if (!raw) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    forgetSession(draftKey);
    return null;
  }
  // Three outcomes, and only the first leaves the value where it is: a draft addressed to
  // another surface is that surface's to collect, while one addressed to nobody and one
  // addressed to us but malformed are both ours to throw away.
  const tag = (parsed as { surface?: unknown } | null)?.surface;
  if (typeof tag === 'string' && tag !== surface) return null;
  forgetSession(draftKey);
  return isDraftFor(parsed, surface) ? parsed : null;
}

function base64url(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
}

export async function beginProviderReauthentication(
  provider: string,
  returnTo: ReauthReturnPath,
  draft?: ReauthDraft,
): Promise<void> {
  if (!/^[a-z][a-z0-9_-]{1,31}$/.test(provider)) throw new Error('invalid provider');
  const verifierBytes = new Uint8Array(32);
  crypto.getRandomValues(verifierBytes);
  const verifier = base64url(verifierBytes);
  const challenge = base64url(
    new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))),
  );
  // Deliberately unguarded, unlike every read in this module. Without a stored verifier
  // the return leg can never complete, so swallowing a storage failure here would spend a
  // full-page navigation to arrive at "attempt expired" — failing before the trip says
  // the same thing and costs the member nothing.
  sessionStorage.setItem(verifierKey, verifier);
  sessionStorage.setItem(returnKey, returnTo);
  // Written or cleared, never left alone: a draft from an abandoned earlier trip would
  // otherwise ride along with this one and reopen a surface nobody asked for.
  if (draft) sessionStorage.setItem(draftKey, JSON.stringify(draft));
  else forgetSession(draftKey);
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
  const verifier = readSession(verifierKey);
  forgetSession(verifierKey);
  try {
    if (!verifier) throw new Error('reauthentication attempt expired');
    // Only the parsed INSTANT is kept, never the string the endpoint answered with. The
    // proof itself lives in an HttpOnly cookie this code cannot read; all that is recorded
    // here is when it stops being valid, so nothing sensitive reaches storage — and
    // reducing it to a number at the point of writing says that in the code rather than
    // only in a comment. An unparseable answer records nothing, which reads downstream as
    // "no proof held".
    const expiresAt = Date.parse(await api.exchangeOAuthReauthentication(code, verifier));
    if (Number.isFinite(expiresAt)) sessionStorage.setItem(expiryKey, String(expiresAt));
  } catch (e) {
    // A trip that does not end in a proof leaves nothing behind. The draft only means
    // anything to the surface that is about to be resumed, and there will be no resuming —
    // keeping it would reopen that surface on some unrelated later visit instead.
    //
    // Through `forget`, not a bare `removeItem`: a throwing storage would otherwise raise
    // from inside this handler and replace the failure the caller actually needs to see.
    forgetSession(draftKey);
    forgetSession(returnKey);
    forgetSession(expiryKey);
    throw e;
  }
  const storedTarget = readSession(returnKey);
  forgetSession(returnKey);
  return storedTarget === '/my/api-keys' ? storedTarget : '/my/security';
}
