import { isCrawler, regionFromCountry } from '$lib/geoScope';
import type { RequestHandler } from './$types';

// The visitor's macro-region, derived from Cloudflare's `CF-IPCountry`, for the
// jobs feed's opening scope.
//
// It is an endpoint rather than page data on purpose. A server `load` returning the
// region would serialize it into the document SvelteKit ships, so the HTML would
// differ by country — and that HTML is held by a shared cache keyed on the URL
// alone. Answering here instead keeps every cached page identical for everyone and
// confines the per-visitor part to a response that may not be stored at all.
//
// The client calls this only when the guess is actually in play (no geography in
// the URL, no stored filter set, marker unset), so it costs one small request on a
// genuine first visit and nothing afterwards.
//
// Shape is a plain `{ region }` rather than the catalogue API's `{ data }` envelope:
// that convention belongs to the Go service under /api/, and borrowing it here would
// suggest this is part of it.
export const GET: RequestHandler = ({ request, setHeaders }) => {
  // Never stored — not by the browser, not by a shared cache. Without this the
  // response is exactly the kind a CDN is happy to hold and replay to the next
  // visitor, who is in a different country.
  setHeaders({ 'cache-control': 'private, no-store' });

  // Most traffic here is automated. A rendering crawler handed a region would index
  // a feed scoped to wherever its exit address sits — a scope no canonical URL
  // describes and no human asked for. The check lives here because the client
  // cannot know what it is, and a check the client could skip is not a check.
  if (isCrawler(request.headers.get('user-agent'))) {
    return Response.json({ region: null });
  }

  return Response.json({ region: regionFromCountry(request.headers.get('cf-ipcountry')) });
};
