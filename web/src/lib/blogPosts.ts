// Blog post discovery. Posts are `.svx` files under web/src/posts/, compiled by
// mdsvex at build time (see svelte.config.js). This is the single source of truth
// for which posts exist — the /blog index, the post page, the sitemap, and the RSS
// feed all read through `listPosts`/`getPost`, so draft-filtering and ordering
// can't drift. The pure helpers it builds on live in `./blog` (unit-tested there);
// this module is exercised by the build and the routes because it evaluates
// `import.meta.glob` over `.svx`, which needs the mdsvex plugin.
import type { Component } from 'svelte';
import { parseFrontmatter, selectPosts, slugFromPath, type PostMeta } from './blog';

/** A post with its compiled body component, for the post page. */
export type PostWithBody = {
  meta: PostMeta;
  component: Component;
};

// Drafts are visible only in dev, so authors can preview locally while prod hides
// them from every surface. A single gate keeps index/post/sitemap/rss in agreement.
const INCLUDE_DRAFTS = import.meta.env.DEV;

// One eager glob for both the component (`default`) and its frontmatter
// (`metadata`). A named-export glob (`{ import: 'metadata' }`) would trip Vite's
// dev dep-scanner, which parses `.svx` without mdsvex and can't find the export;
// reading `metadata` off the namespace is opaque to the scanner. The post count
// is tiny, so eagerly pulling the bodies for a listing is negligible.
const postModules = import.meta.glob<{ default: Component; metadata: Record<string, unknown> }>(
  '/src/posts/*.svx',
  { eager: true },
);

function allPosts(): PostMeta[] {
  return Object.entries(postModules).map(([path, mod]) => parseFrontmatter(mod.metadata ?? {}, path));
}

/** Published posts (drafts included only in dev), newest-first — for the index,
 *  sitemap, and RSS. */
export function listPosts(): PostMeta[] {
  return selectPosts(allPosts(), { includeDrafts: INCLUDE_DRAFTS });
}

/** Load a single post's metadata + body component by slug. Returns `undefined`
 *  when no post matches, or when the post is a draft outside dev (so the post
 *  page and OG route 404 on drafts in prod). */
export function getPost(slug: string): PostWithBody | undefined {
  for (const [path, mod] of Object.entries(postModules)) {
    if (slugFromPath(path) !== slug) continue;
    const meta = parseFrontmatter(mod.metadata ?? {}, path);
    if (meta.draft && !INCLUDE_DRAFTS) return undefined;
    return { meta, component: mod.default };
  }
  return undefined;
}
