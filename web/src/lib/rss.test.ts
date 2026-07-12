import { describe, it, expect } from 'vitest';
import { blogRssXml } from './rss';
import type { PostMeta } from './blog';

const ORIGIN = 'https://freehire.dev';

function post(overrides: Partial<PostMeta> = {}): PostMeta {
  return {
    slug: 'launch',
    title: 'Launch',
    date: '2026-01-15',
    summary: 'We shipped it.',
    type: 'changelog',
    tags: [],
    draft: false,
    ...overrides,
  };
}

describe('blogRssXml', () => {
  it('renders a valid RSS 2.0 channel with one item per post, in the given order', () => {
    const xml = blogRssXml([post({ slug: 'b', title: 'B' }), post({ slug: 'a', title: 'A' })], ORIGIN);
    expect(xml).toContain('<rss version="2.0">');
    expect(xml).toContain('<channel>');
    expect(xml.match(/<item>/g)?.length).toBe(2);
    // Order is preserved (the caller passes newest-first).
    expect(xml.indexOf('<title>B</title>')).toBeLessThan(xml.indexOf('<title>A</title>'));
  });

  it('builds absolute link + guid and an RFC-822 pubDate from the ISO date', () => {
    const xml = blogRssXml([post({ slug: 'launch', date: '2026-01-15' })], ORIGIN);
    expect(xml).toContain('<link>https://freehire.dev/blog/launch</link>');
    expect(xml).toContain('<guid isPermaLink="true">https://freehire.dev/blog/launch</guid>');
    expect(xml).toContain('<pubDate>Thu, 15 Jan 2026 00:00:00 GMT</pubDate>');
  });

  it('escapes XML-special characters in title and summary', () => {
    const xml = blogRssXml([post({ title: 'A & B <ok>', summary: '"quote" & more' })], ORIGIN);
    expect(xml).toContain('<title>A &amp; B &lt;ok&gt;</title>');
    expect(xml).toContain('&quot;quote&quot; &amp; more');
    expect(xml).not.toContain('<ok>');
  });

  it('renders an empty channel (no items) when there are no posts', () => {
    const xml = blogRssXml([], ORIGIN);
    expect(xml).toContain('<channel>');
    expect(xml).not.toContain('<item>');
  });
});
