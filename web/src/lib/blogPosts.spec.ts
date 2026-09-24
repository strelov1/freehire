import { describe, it, expect } from 'vitest';
import { listPosts, getPost } from './blogPosts';

// Every real post, through the real loader.
//
// blog.test.ts covers parseFrontmatter on objects written by hand, which proves the
// validator works on input someone typed into the test. It cannot say whether the posts
// on disk actually parse, and nothing else did either: mdsvex compiles a post's
// BODY at build time, but its frontmatter is only read when `listPosts` runs — i.e. when
// a reader opens /blog. So a malformed post built green, deployed green, and 500'd the
// whole index on the first request.
//
// That is how `summary: ... no API key: an agent reads ...` reached production on
// 2026-09-23. A YAML plain scalar cannot contain ": ", so the frontmatter block failed to
// parse, `date` arrived undefined, and parseFrontmatter threw for every surface that
// lists posts — index, sitemap and RSS at once, not just the one bad post.
//
// This lives in the `components` project (a `.spec.ts`) rather than `unit` because it has
// to go through `import.meta.glob` over `.svx`, which needs the Svelte plugin and mdsvex.
// Importing the real module is the point: a copy of the loader here could pass while the
// one prod runs throws.
describe('every post on disk', () => {
  const posts = listPosts();

  it('loads at all — a post whose frontmatter cannot be parsed throws here, not in prod', () => {
    expect(posts.length).toBeGreaterThan(0);
  });

  // There are deliberately no per-post assertions on title, summary, date or type.
  // parseFrontmatter already rejects a blank or malformed value for every one of them, and
  // allPosts runs it over each file, so the call above returning at all IS that assertion.
  // Restating them here would test blog.ts's validator a second time and prove nothing
  // about the posts.

  it('serves each listed post by its own slug', () => {
    // listPosts and getPost reach the same modules by different routes — selectPosts versus
    // a slugFromPath scan, each with its own draft gate — and blogPosts.ts exists to keep
    // index, post page, sitemap and RSS agreeing. A post in the listing but unreachable by
    // slug is a 404 from a link we published ourselves.
    for (const post of posts) {
      expect(getPost(post.slug), `getPost(${post.slug})`).toBeTruthy();
    }
  });
});
