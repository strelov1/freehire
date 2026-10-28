// The Pro monthly price as the backend's plan matrix states it, for feature landings that quote it.
import { formatMinorUnits } from './money';
import type { PlansMatrix, PublicPrice } from './types';

export function proMonthlyPrice(plans: PlansMatrix | null): PublicPrice | null {
  return plans?.prices.find((p) => p.tier === 'pro' && p.interval === 'month') ?? null;
}

/** "$5" rather than "$5.00": a whole price reads as one on a call to action. Formatted in
 *  English like the landings that show it: the server writes the label and the browser
 *  hydrates it, and a runtime-locale format would render "5,00 $" for a German browser. */
export function priceLabel(p: PublicPrice): string {
  return formatMinorUnits(p.amount_cents, p.currency, 'en-US').replace(/\.00$/, '');
}
