/** Render an amount the payment provider gave us in MINOR UNITS.
 *
 *  How many minor units make one unit is a property of the CURRENCY, not a constant: 100 for
 *  dollars and euros, 1 for yen, 1000 for dinars. Dividing by 100 unconditionally renders
 *  ¥1000 as ¥10 — an error of a hundredfold, on a screen about somebody's money, in their
 *  favour and therefore unlikely to be reported.
 *
 *  `Intl` already knows each currency's exponent, so it is asked rather than tabulated here:
 *  a table would be a second source of truth about currencies, and it would be wrong for
 *  whichever one nobody thought of.
 *
 *  It lives here rather than beside either caller because both the pricing page and the
 *  subscription section render money, and two copies of a money rule is the shape that
 *  eventually shows two different prices for one product. */
/** The currencies Stripe encodes with two implied decimals even though they HAVE none.
 *
 *  This is not a fact about the currency — Intl is right that ISK and UGX take no
 *  fraction — it is a fact about the PROVIDER, which requires their amounts as a multiple
 *  of 100 and hands back 500 for five krónur. Intl cannot know that, so it is the one thing
 *  the comment above is wrong to leave to it. Getting it from Intl anyway prints kr 500 for
 *  a kr 5 charge: a hundredfold error against the customer, on the receipt list. */
const STRIPE_HUNDREDFOLD = new Set(['ISK', 'UGX']);

export function formatMinorUnits(minor: number, currency: string): string {
  const code = (currency || 'usd').toUpperCase();
  const fmt = new Intl.NumberFormat(undefined, { style: 'currency', currency: code });
  const exponent = STRIPE_HUNDREDFOLD.has(code)
    ? 2
    : (fmt.resolvedOptions().maximumFractionDigits ?? 2);
  return fmt.format(minor / 10 ** exponent);
}
