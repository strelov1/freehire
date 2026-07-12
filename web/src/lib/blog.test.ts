import { describe, it, expect } from 'vitest';
import { slugFromPath, parseFrontmatter, selectPosts, type PostMeta } from './blog';

describe('slugFromPath', () => {
  it('derives the slug from the file basename without extension', () => {
    expect(slugFromPath('/src/posts/hello-world.svx')).toBe('hello-world');
  });

  it('handles a bare filename', () => {
    expect(slugFromPath('2026-01-01-launch.svx')).toBe('2026-01-01-launch');
  });
});

describe('parseFrontmatter', () => {
  const full = {
    title: 'Launch',
    date: '2026-01-15',
    summary: 'We shipped it.',
    type: 'article',
    tags: ['product', 'launch'],
    draft: false,
  };

  it('parses a well-formed post into the typed shape with slug from the path', () => {
    const post = parseFrontmatter(full, '/src/posts/launch.svx');
    expect(post).toEqual<PostMeta>({
      slug: 'launch',
      title: 'Launch',
      date: '2026-01-15',
      summary: 'We shipped it.',
      type: 'article',
      tags: ['product', 'launch'],
      draft: false,
    });
  });

  it('defaults type to changelog, tags to [], draft to false when omitted', () => {
    const post = parseFrontmatter(
      { title: 'Note', date: '2026-02-01', summary: 'Small fix.' },
      '/src/posts/note.svx',
    );
    expect(post.type).toBe('changelog');
    expect(post.tags).toEqual([]);
    expect(post.draft).toBe(false);
  });

  it.each(['title', 'date', 'summary'])('throws naming the file when %s is missing', (field) => {
    const raw = { ...full };
    delete (raw as Record<string, unknown>)[field];
    expect(() => parseFrontmatter(raw, '/src/posts/broken.svx')).toThrow(/broken\.svx/);
  });

  it('throws naming the file for a type outside the enum', () => {
    expect(() =>
      parseFrontmatter({ ...full, type: 'news' }, '/src/posts/bad-type.svx'),
    ).toThrow(/bad-type\.svx/);
  });

  it.each(['Jan 15 2026', '2026-13-40', '2026/01/15', '15-01-2026'])(
    'throws naming the file for a non-ISO date %s',
    (date) => {
      expect(() => parseFrontmatter({ ...full, date }, '/src/posts/bad-date.svx')).toThrow(
        /bad-date\.svx/,
      );
    },
  );

  it('throws naming the file when tags is present but not a list', () => {
    expect(() =>
      parseFrontmatter({ ...full, tags: 'product' }, '/src/posts/bad-tags.svx'),
    ).toThrow(/bad-tags\.svx/);
  });
});

describe('selectPosts', () => {
  const posts: PostMeta[] = [
    { slug: 'a', title: 'A', date: '2026-01-01', summary: '', type: 'changelog', tags: [], draft: false },
    { slug: 'b', title: 'B', date: '2026-03-01', summary: '', type: 'article', tags: [], draft: false },
    { slug: 'c', title: 'C', date: '2026-02-01', summary: '', type: 'changelog', tags: [], draft: true },
  ];

  it('excludes drafts and sorts newest-first when includeDrafts is false', () => {
    const result = selectPosts(posts, { includeDrafts: false });
    expect(result.map((p) => p.slug)).toEqual(['b', 'a']);
  });

  it('includes drafts (still newest-first) when includeDrafts is true', () => {
    const result = selectPosts(posts, { includeDrafts: true });
    expect(result.map((p) => p.slug)).toEqual(['b', 'c', 'a']);
  });

  it('does not mutate the input array', () => {
    const snapshot = posts.map((p) => p.slug);
    selectPosts(posts, { includeDrafts: true });
    expect(posts.map((p) => p.slug)).toEqual(snapshot);
  });
});
