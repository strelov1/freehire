import { error } from '@sveltejs/kit';
import { getPost } from '$lib/blogPosts';
import { buildBlogCard } from '$lib/server/og/blog';
import { loadOgFonts } from '$lib/server/og/fonts';
import { renderMarkupPng } from '$lib/server/og/render';
import type { RequestHandler } from './$types';

// Renders the per-post 1200×630 Open Graph card on demand. Resolves the post by
// the same slug as the page (unknown — or a draft outside dev — → 404, no image).
// Cached for an hour with a day of stale-while-revalidate; post content is static.
export const GET: RequestHandler = async ({ params }) => {
  const post = getPost(params.slug);
  if (!post) error(404, 'Post not found');

  const fonts = await loadOgFonts();
  const png = await renderMarkupPng(buildBlogCard(post.meta), fonts);

  return new Response(png, {
    headers: {
      'content-type': 'image/png',
      'cache-control': 'public, max-age=3600, stale-while-revalidate=86400',
    },
  });
};
