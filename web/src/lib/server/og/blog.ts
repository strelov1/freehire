// Builds the HTML for a blog post's Open Graph card (light, 1200×630): a small
// type + date eyebrow, the post title, and its summary, over the shared brand
// footer. Pure and synchronous (PostMeta → HTML string for satori), mirroring
// buildCompanyCard. satori constraint: flexbox-only layout, and any element with
// more than one child declares `display:flex`.
import type { PostMeta } from '$lib/blog';
import { OG_HEIGHT, OG_WIDTH, brandFooter, esc } from './shared';

// UTC so the rendered date matches the ISO frontmatter regardless of server tz.
const dateFmt = new Intl.DateTimeFormat('en-US', { dateStyle: 'long', timeZone: 'UTC' });

/** Builds the card HTML for a blog `post`. */
export function buildBlogCard(post: PostMeta): string {
  const eyebrow = `${post.type.toUpperCase()} · ${dateFmt.format(new Date(post.date))}`;

  return `
<div style="display:flex;flex-direction:column;justify-content:space-between;width:${OG_WIDTH}px;height:${OG_HEIGHT}px;padding:64px 72px;background:#ffffff;color:#0a0a0a;font-family:Inter">
  <div style="display:flex;flex-direction:column;gap:28px">
    <div style="display:flex;font-size:26px;font-weight:600;letter-spacing:0.06em;color:#737373">${esc(eyebrow)}</div>
    <div style="display:flex;font-size:64px;font-weight:700;letter-spacing:-0.03em;overflow:hidden">${esc(post.title)}</div>
    <div style="display:flex;font-size:30px;color:#404040;overflow:hidden">${esc(post.summary)}</div>
  </div>
  ${brandFooter()}
</div>`.trim();
}
