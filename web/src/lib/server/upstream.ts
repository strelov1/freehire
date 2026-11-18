import { error } from '@sveltejs/kit';

import { ApiError } from '#lib/api.js';

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
  if (e instanceof ApiError) {
    switch (e.status) {
      case 429:
        return error(429, 'Too many requests');
      case 503:
        return error(503, 'Service unavailable');
      case 504:
        // createApi uses 504 when the SSR deadline aborts an unanswered request.
        return error(504, 'Gateway timeout');
    }
  }
  throw e;
}
