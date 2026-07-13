import { listPosts } from '$lib/blogPosts';
import type { PageLoad } from './$types';

// Published posts (metadata only), newest-first. Universal load: the post data is
// static and bundled, so it resolves the same on server and client.
export const load: PageLoad = () => ({ posts: listPosts() });
