// What the edge's view of a visitor implies about whether they can pay us.
//
// Pure by design, and free of SvelteKit and of the DOM, like `geoScope.ts` beside it:
// nothing here reads a header, a store, or a URL — callers pass in what they already
// hold, and the unit tests run it in plain Node.

/** The country our own Stripe account is registered in.
 *
 *  This, not Brazil-the-country, is the reason the check exists. A card issued where
 *  the business is registered makes the payment domestic: no border is crossed, so
 *  there is nothing to convert, and a domestic payment must be in the local currency.
 *  Our prices are in USD, so Stripe refuses those cards with `currency_not_supported`.
 *
 *  Named for the account rather than hardcoded at the call site so that whoever moves
 *  the account one day changes the fact, not a coincidence. */
const ACCOUNT_COUNTRY = 'br';

/** The same country as a reader would name it, for the sentence that has to tell them
 *  which card to reach for instead. Exported from here so the fact and the word for it
 *  move together: a page holding its own copy would still say "Brazil" long after the
 *  account had moved. */
export const ACCOUNT_COUNTRY_NAME = 'Brazil';

/** Whether a visitor placed in this country will find their card refused.
 *
 *  A guess, and deliberately a soft one. It reports where the person is, which is not
 *  the same as which card they hold: somebody in Brazil may be carrying a foreign card,
 *  and a Brazilian abroad is not placed here at all. Callers may use it to say
 *  something and must not use it to withhold anything.
 *
 *  Anything unplaceable — a missing header, blank, or one of Cloudflare's reserved
 *  `XX`/`T1` — reads as payable. Warning somebody we could not locate spends a real
 *  reader's attention on a guess we never made. */
export function cardsUnsupportedFrom(country: string | null | undefined): boolean {
  return country?.trim().toLowerCase() === ACCOUNT_COUNTRY;
}
