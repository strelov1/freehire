import { error } from '@sveltejs/kit';

import { ApiError } from '$lib/api';

/** Re-throw an API failure a `load` could not handle, mapping the statuses that mean
 *  something to the CLIENT onto the response instead of letting them become a 500.
 *
 *  SvelteKit turns an unhandled throw in `load` into a 500, so before this every upstream
 *  failure looked to a visitor — and to a crawler — like a broken page. The two differ in
 *  what they invite: a 500 says the url is defective and is worth dropping, a 429 says come
 *  back later and costs the page nothing. On a public catalogue that distinction is most of
 *  the value of answering at all.
 *
 *  Anything else still becomes a 500. A genuine defect should look like one. */
export function rethrowUpstream(e: unknown): never {
  if (e instanceof ApiError && (e.status === 429 || e.status === 503)) {
    error(e.status, e.status === 429 ? 'Too many requests' : 'Service unavailable');
  }
  throw e;
}
