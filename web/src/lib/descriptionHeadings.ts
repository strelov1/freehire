// A posting's description is a FRAGMENT embedded in a page that has its own outline: the
// job page's <h1> is the title and its <h2>s are "Summary" and "Skills". Source HTML keeps
// whatever headings the employer's ATS wrote, so an <h1> in the body gave the page a second
// top-level heading, and body <h2>s sat beside the page's own sections as their equals.
//
// The level is a property of where the fragment is EMBEDDED, not of the posting, which is
// why this runs at render time rather than in the ingest sanitizer
// (internal/platform/htmltext): every stored row is fixed at once, with no backfill, and the
// API keeps handing other consumers the headings the source wrote.
//
// The shift is relative, not a clamp: the body's shallowest heading becomes `top` and every
// other heading moves by the same amount, so an h1/h2 pair stays a parent and a child
// instead of collapsing into two h3s. Levels past h6 clamp there, since HTML has no h7.

const headingTag = /<(\/?)h([1-6])(?=[\s>/])/gi;

/** Shifts every heading in `html` so its shallowest one is `<h{top}>`, preserving the
 *  relative depth of the rest. A body whose shallowest heading is already at or below
 *  `top` is returned unchanged, so the transform is idempotent. */
export function demoteHeadings(html: string, top = 3): string {
  let shallowest = 7;
  for (const m of html.matchAll(headingTag)) {
    shallowest = Math.min(shallowest, Number(m[2]));
  }
  const shift = top - shallowest;
  if (shift <= 0) return html;
  return html.replace(
    headingTag,
    (_, slash: string, level: string) => `<${slash}h${Math.min(Number(level) + shift, 6)}`,
  );
}
