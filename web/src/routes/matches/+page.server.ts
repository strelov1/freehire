import { error } from '@sveltejs/kit';
import { PUBLIC_MATCHES } from '$app/env/public';
import { matchesEnabled } from '#lib/features.js';
import { serverApi } from '#lib/server/api.js';
import { proMonthlyPrice } from '#lib/proPrice.js';
import type { PageServerLoad } from './$types';

// The feed is personal and dark until it ships: off, the page does not exist; signed out,
// the explainer says what it is and its Sign up comes back here.
export const load: PageServerLoad = async ({ parent, fetch }) => {
  if (!matchesEnabled({ PUBLIC_MATCHES })) error(404, 'Not found');
  const { user } = await parent();
  if (!user) return { signedOut: true, paid: false, price: null };
  if ((user.tier ?? 'free') !== 'free') return { paid: true, price: null };
  // A failed price load drops the price; the button falls back to /pricing.
  try {
    return { paid: false, price: proMonthlyPrice(await serverApi(fetch).plans()) };
  } catch {
    return { paid: false, price: null };
  }
};
