import { listPosts } from '#lib/blogPosts.js';
import { blogRssXml } from '#lib/rss.js';
import type { RequestHandler } from './$types';

// RSS 2.0 feed of published posts (newest-first, drafts excluded via listPosts).
export const GET: RequestHandler = ({ url }) => {
  return new Response(blogRssXml(listPosts(), url.origin), {
    headers: {
      'content-type': 'application/rss+xml; charset=utf-8',
      'cache-control': 'public, max-age=3600',
    },
  });
};
