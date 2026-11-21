import { serverApi } from '#lib/server/api.js';
import { proMonthlyPrice } from '#lib/proPrice.js';
import type { PageServerLoad } from './$types';

// The price is loaded on the server so the call to action carries it in the HTML. It
// reads no cookie: this page is cached by URL alone. A failed load drops the price, and
// the button falls back to /pricing — a missing price is better than a stale one.
export const load: PageServerLoad = async ({ fetch }) => {
  try {
    return { price: proMonthlyPrice(await serverApi(fetch).plans()) };
  } catch {
    return { price: null };
  }
};
