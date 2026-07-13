// Pure RSS 2.0 builder for the blog feed. Kept dependency-free (mirrors the
// XML-escaping discipline in ./sitemap) and separate from the route so it runs
// under the plain-Node vitest config. The route passes `listPosts()` (already
// draft-filtered and newest-first); this renders whatever order it's given.
import type { PostMeta } from './blog';

function escapeXml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function item(post: PostMeta, origin: string): string {
  const url = `${origin}/blog/${post.slug}`;
  return `    <item>
      <title>${escapeXml(post.title)}</title>
      <link>${escapeXml(url)}</link>
      <guid isPermaLink="true">${escapeXml(url)}</guid>
      <pubDate>${new Date(post.date).toUTCString()}</pubDate>
      <description>${escapeXml(post.summary)}</description>
    </item>`;
}

/** An RSS 2.0 document for the blog. `origin` is the absolute site origin. */
export function blogRssXml(posts: PostMeta[], origin: string): string {
  const items = posts.map((post) => item(post, origin)).join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>freehire blog</title>
    <link>${escapeXml(`${origin}/blog`)}</link>
    <description>Product updates and articles from freehire.</description>
${items}
  </channel>
</rss>
`;
}
