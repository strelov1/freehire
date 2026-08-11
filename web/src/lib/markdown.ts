// Rendering untrusted model prose to markup (assistant answers, fit verdict, …).
//
// freehire is a PUBLIC app and model output is untrusted, so marked's output is run
// through DOMPurify before it reaches `{@html}`. `isomorphic-dompurify` is SSR-safe,
// so the guard holds even though some surfaces paint client-side only — and it is what
// lets this module be unit-tested under the node-environment vitest config.

import { Marked } from 'marked';
import DOMPurify from 'isomorphic-dompurify';

const md = new Marked({ gfm: true, breaks: true });

// An explicit prose allowlist, not a list of forbidden tags. A denylist over
// DOMPurify's permissive default has to stay ahead of every element that can fetch,
// and the ones already shipping are easy to miss — `<svg><use href>`, `<object>`,
// `<embed>` and `<video poster>` all issue requests and none look like media at a
// glance. This mirrors `newDescriptionPolicy()` in internal/sources/sanitize.go,
// which made the same call for job description HTML: same class of untrusted input,
// same reason. Notably absent: `img` and `input` (GFM task lists render a checkbox —
// a form control, and losing it costs one glyph).
const ALLOWED_TAGS = [
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'p', 'br', 'hr', 'blockquote', 'pre', 'code', 'div', 'span',
  'ul', 'ol', 'li', 'dl', 'dt', 'dd',
  'table', 'thead', 'tbody', 'tr', 'th', 'td',
  'strong', 'em', 'b', 'i', 'u', 'del', 's',
  'a',
];

// `target`/`rel` are here because the hook below writes them; `align` is what GFM
// table alignment compiles to.
const ALLOWED_ATTR = ['href', 'target', 'rel', 'align'];

// Schemes, narrowed to match bluemonday's AllowStandardURLs() on the backend side:
// http, https, mailto — and relative URLs, which are same-origin by definition and so
// cannot carry anything off the site. The two trailing alternatives are what admit them
// (a leading `/`, `#` or `.`, or a path segment not followed by a colon); without them a
// link like `[profile](/my/profile)` silently loses its href. `javascript:` and `data:`
// still fail, because each is a token that IS followed by a colon and is not listed.
// The `-` sits last inside each character class so it is a literal, not a range.
const ALLOWED_URI_REGEXP = /^(?:(?:https?|mailto):|[^a-z]|[a-z+.-]+(?:[^a-z+.:-]|$))/i;

// Open links the model writes in a new tab so clicking one never navigates the chat
// away. DOMPurify's hook registry is global, so the hook is added and removed around
// each call rather than installed once — and removed in a `finally`, because a throw
// inside `sanitize` would otherwise leave it applying to every other consumer.
function openLinksInNewTab(node: Element) {
  if (node.tagName === 'A') {
    node.setAttribute('target', '_blank');
    node.setAttribute('rel', 'noopener noreferrer');
  }
}

/** Render one markdown string from the model to sanitized HTML for `{@html}`. */
export function renderMarkdown(text: string): string {
  const html = md.parse(text, { async: false }) as string;
  DOMPurify.addHook('afterSanitizeAttributes', openLinksInNewTab);
  try {
    return DOMPurify.sanitize(html, {
      ALLOWED_TAGS,
      ALLOWED_ATTR,
      ALLOWED_URI_REGEXP,
      ALLOW_DATA_ATTR: false,
    });
  } finally {
    DOMPurify.removeHook('afterSanitizeAttributes');
  }
}
