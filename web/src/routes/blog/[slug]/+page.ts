import { error } from '@sveltejs/kit';
import { getPost } from '$lib/blogPosts';
import type { PageLoad } from './$types';

// Universal load so the compiled body component (not JSON-serializable) can pass
// straight through. `getPost` returns undefined for an unknown slug — or a draft
// outside dev — which becomes a 404.
export const load: PageLoad = ({ params }) => {
  const post = getPost(params.slug);
  if (!post) error(404, 'Post not found');
  return { meta: post.meta, component: post.component };
};
