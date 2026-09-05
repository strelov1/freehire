// Referral and promo capture: reading a code out of a link and deciding whether it may be
// stored. Pure functions, no SvelteKit imports, so this unit-tests in plain node; the
// request hook that applies the decision lives in hooks.server.ts.

/** The cookie carrying the referrer's invite code from the link to registration.
 *
 *  Its name is also a constant in `internal/identity/promo` — the Go side reads this cookie
 *  in both registration paths. A rename on one side alone would simply stop attributing,
 *  and nothing would fail, so the two names are stated in each other's comments. */
export const REF_COOKIE = 'fh_ref';

/** The cookie carrying a promo code arriving in a link, so the pricing page can prefill
 *  the field instead of asking somebody to retype what they just clicked. */
export const PROMO_COOKIE = 'fh_promo';

/** How long a captured code survives. Long enough that a link shared on a Friday still
 *  works when somebody signs up the following month; short enough that it is not a
 *  permanent marker on a browser. */
export const CAPTURE_MAX_AGE_SECONDS = 30 * 24 * 60 * 60;

/** What an invite code can look like — mirrors the CHECK constraint on `invite_codes`. */
const REF_SHAPE = /^[A-Za-z0-9_-]{16,64}$/;

/** What a promo code can look like — mirrors the CHECK constraint on `promo_codes`. */
const PROMO_SHAPE = /^[A-Z0-9]{4,32}$/;

/** A code to store, or nothing. */
export type Capture = { name: string; value: string } | null;

/** Decide whether to store an invite code found in a link.
 *
 *  FIRST TOUCHER WINS: an existing cookie is never overwritten. Otherwise a second link —
 *  or a link somebody talks a pending invitee into opening — would take over an
 *  attribution that has already been earned. */
export function captureRef(code: string | null, existing: string | undefined): Capture {
  if (!code || existing) return null;
  if (!REF_SHAPE.test(code)) return null;
  return { name: REF_COOKIE, value: code };
}

/** Decide whether to store a promo code found in a link.
 *
 *  Unlike the invite code, a later one DOES win. This cookie only prefills a form field;
 *  nothing is attributed by it and nothing is earned from it, so the most recent offer
 *  somebody clicked is the one they meant. */
export function capturePromo(code: string | null): Capture {
  if (!code) return null;
  const folded = code.trim().toUpperCase();
  if (!PROMO_SHAPE.test(folded)) return null;
  return { name: PROMO_COOKIE, value: folded };
}
