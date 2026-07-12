// Pure blog helpers: post types + frontmatter validation + ordering. Kept free of
// `import.meta.glob` (that lives in blogPosts.ts) so this module runs under the
// plain-Node vitest config — same discipline as facetModel.ts. The /blog index,
// the post page, the sitemap, and the RSS feed all normalize posts through here,
// so draft-filtering and ordering can't drift.

/** A post is either a short release note (`changelog`) or a long-form `article`.
 *  One feed, two tiers — see the blog-changelog design (D6). */
export type PostType = 'changelog' | 'article';

const POST_TYPES: readonly PostType[] = ['changelog', 'article'];

/** Typed, validated post frontmatter plus its file-derived slug. */
export type PostMeta = {
  slug: string;
  title: string;
  date: string; // ISO YYYY-MM-DD; lexicographically sortable
  summary: string;
  type: PostType;
  tags: string[];
  draft: boolean;
};

/** Derive the slug from a glob key like `/src/posts/hello-world.svx`. */
export function slugFromPath(filePath: string): string {
  const base = filePath.split('/').pop() ?? filePath;
  return base.replace(/\.svx$/, '');
}

function isPostType(value: unknown): value is PostType {
  return typeof value === 'string' && (POST_TYPES as readonly string[]).includes(value);
}

// A real ISO calendar date: the `YYYY-MM-DD` shape AND an actual day (rejects
// 2026-13-40). Round-tripping through Date and re-formatting catches overflow.
function isIsoDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const d = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === value;
}

/** Validate and normalize one post's raw frontmatter into a `PostMeta`. Throws,
 *  naming the offending file, on a missing required field or an out-of-enum
 *  `type` — a loud build failure beats shipping a post with blank metadata. */
export function parseFrontmatter(raw: Record<string, unknown>, filePath: string): PostMeta {
  const required = (key: 'title' | 'date' | 'summary'): string => {
    const value = raw[key];
    if (typeof value !== 'string' || value.trim() === '') {
      throw new Error(`blog post ${filePath}: missing or empty required field "${key}"`);
    }
    return value;
  };

  const date = required('date');
  if (!isIsoDate(date)) {
    throw new Error(`blog post ${filePath}: invalid date "${date}" (expected ISO YYYY-MM-DD)`);
  }

  let type: PostType = 'changelog';
  if (raw.type !== undefined) {
    if (!isPostType(raw.type)) {
      throw new Error(
        `blog post ${filePath}: invalid type "${String(raw.type)}" (expected ${POST_TYPES.join(' | ')})`,
      );
    }
    type = raw.type;
  }

  if (raw.tags !== undefined && !Array.isArray(raw.tags)) {
    throw new Error(`blog post ${filePath}: tags must be a list`);
  }

  return {
    slug: slugFromPath(filePath),
    title: required('title'),
    date,
    summary: required('summary'),
    type,
    tags: Array.isArray(raw.tags) ? raw.tags.map(String) : [],
    draft: raw.draft === true,
  };
}

/** Filter drafts (unless `includeDrafts`) and order newest-first. Non-mutating. */
export function selectPosts(posts: PostMeta[], { includeDrafts }: { includeDrafts: boolean }): PostMeta[] {
  return posts
    .filter((post) => includeDrafts || !post.draft)
    .toSorted((a, b) => b.date.localeCompare(a.date));
}
