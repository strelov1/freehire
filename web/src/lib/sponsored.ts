// Pure money helpers for sponsored campaigns. The API speaks USD cents; the UI speaks dollars.

export const usd = (cents: number, locale = 'en') =>
  new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD' }).format(cents / 100);

/** "1.25" or "$1,25" -> 125. null for blank or unparseable input; the server enforces the minimums. */
export function dollarsToCents(input: string): number | null {
  const n = Number(input.trim().replace(/^\$/, '').replace(',', '.'));
  return input.trim() !== '' && Number.isFinite(n) && n > 0 ? Math.round(n * 100) : null;
}
