import type { RequestHandler } from './$types';

// A real robots file (not the SPA shell): allow crawling the public pages, keep
// personal pages out, and point at the sitemap.
//
// The wildcard already admits every AI crawler (GPTBot, ClaudeBot, PerplexityBot,
// …), so no per-bot Allow block is listed — a redundant one would only invite
// drift from this rule. llms.txt has no robots directive of its own, so it is
// advertised as a comment, the convention crawlers look for.
export const GET: RequestHandler = ({ url }) => {
  const body = `User-agent: *
Allow: /
Disallow: /my/

Sitemap: ${url.origin}/sitemap.xml
# llms.txt: ${url.origin}/llms.txt
`;
  return new Response(body, {
    headers: {
      'content-type': 'text/plain; charset=utf-8',
      'cache-control': 'public, max-age=86400',
    },
  });
};
