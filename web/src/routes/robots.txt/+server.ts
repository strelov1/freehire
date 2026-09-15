import { robotsBody } from '$lib/robots';
import type { RequestHandler } from './$types';

// A real robots file (not the SPA shell): allow crawling the public pages, keep
// personal pages out, point at the sitemap, and take /api/ away from the search
// crawlers only. The rules and the reasoning behind each live in $lib/robots.ts,
// where a test can read them.
//
// The wildcard already admits every AI crawler (GPTBot, ClaudeBot, PerplexityBot,
// …), so no per-bot Allow block is listed — a redundant one would only invite
// drift from this rule. llms.txt has no robots directive of its own, so it is
// advertised as a comment, the convention crawlers look for.
//
// The API is advertised the same way, and for a self-interested reason: AI
// crawlers are the majority of this site's traffic, and every page they render
// is one unauthenticated JSON call that returns MORE than the HTML does (the
// facets a page only renders are fields in the response). A crawler that takes
// the hint costs us an SSR render instead of thousands and gets better data, so
// the pointer is cheap even at the low odds any given bot reads comments.
// Comments, not directives: robots.txt has no field for "prefer this instead",
// and inventing one would only be ignored by parsers that validate strictly.
export const GET: RequestHandler = ({ url }) => {
  return new Response(robotsBody(url.origin), {
    headers: {
      'content-type': 'text/plain; charset=utf-8',
      'cache-control': 'public, max-age=86400',
    },
  });
};
