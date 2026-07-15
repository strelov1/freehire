// Server-side syntax highlighting for the docs code blocks, via Shiki. Runs in
// `load` (SSR / prerender), so the highlighted HTML is baked into the page and
// there is no client-side highlighter bundle or raw→coloured flash. Dual-theme
// (light + dark) output leans on the `--shiki-light`/`--shiki-dark` CSS vars, so
// one render serves both themes (see the .shiki rules in app.css).

import type { HighlighterGeneric } from 'shiki';

type H = HighlighterGeneric<string, string>;
let highlighterPromise: Promise<H> | null = null;

function getHighlighter(): Promise<H> {
  if (!highlighterPromise) {
    highlighterPromise = import('shiki').then((shiki) =>
      shiki.createHighlighter({
        themes: ['github-light', 'github-dark'],
        langs: ['json', 'bash', 'http'],
      }),
    ) as Promise<H>;
  }
  return highlighterPromise;
}

/** Highlight `code` as `lang`, returning Shiki's dual-theme HTML. Unknown langs
 *  fall back to plain text so a new label never throws. */
export async function highlight(code: string, lang: string): Promise<string> {
  const h = await getHighlighter();
  const resolved = h.getLoadedLanguages().includes(lang) ? lang : 'text';
  return h.codeToHtml(code, {
    lang: resolved,
    themes: { light: 'github-light', dark: 'github-dark' },
    defaultColor: false,
  });
}

/** Pick a Shiki language from a code block's label / content. `curl` blocks are
 *  shell; a body that opens like JSON is JSON; everything else (e.g. an SSE
 *  stream) stays plain text. */
export function langFor(label: string, code: string): string {
  if (label.startsWith('curl') || label === 'bash' || label === 'shell') return 'bash';
  const trimmed = code.trimStart();
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) return 'json';
  return 'text';
}
