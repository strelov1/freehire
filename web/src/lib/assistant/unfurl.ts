// Pure parser that splits an assistant reply into ordered segments so the chat
// can render freehire job links as real job cards while keeping the surrounding
// prose as markdown. Kept out of the Svelte component so the detection is
// unit-testable (vitest) without a DOM — mirroring `chat.ts`/`sessions.ts`.

export type Segment =
  | { kind: 'markdown'; text: string }
  | { kind: 'job'; slug: string; url: string };

// A public slug: alphanumerics + dashes, no leading/trailing dash (so trailing
// punctuation like `.`/`,` is naturally excluded from the capture).
const SLUG = '[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?';
const HOST = '(?:https?:\\/\\/(?:www\\.)?freehire\\.dev)';
// Two forms, matched left-to-right by position:
//  1. a markdown link `[label](…/jobs/<slug>)` (host optional — the parens bound
//     it, so a host-relative link is safe to match), or
//  2. a bare job URL `https://freehire.dev/jobs/<slug>` (host REQUIRED, so a
//     stray `/jobs/…` path in prose is never mistaken for a job link).
// Group 1 = slug from the markdown-link form; group 2 = slug from the bare form.
const JOB_RE = new RegExp(
  `\\[[^\\]]*\\]\\((?:${HOST})?\\/jobs\\/(${SLUG})\\)|${HOST}\\/jobs\\/(${SLUG})`,
  'g',
);

/** Split an assistant reply into ordered markdown/job segments. A freehire job
 *  link — bare or wrapped in a markdown link — becomes a `job` segment (the
 *  markdown label, if any, is dropped: the card shows the real title). Non-job
 *  freehire links and external links are left in the markdown. Always returns at
 *  least one segment. */
export function parseJobSegments(text: string): Segment[] {
  const segments: Segment[] = [];
  let last = 0;
  for (const m of text.matchAll(JOB_RE)) {
    const slug = m[1] ?? m[2];
    const at = m.index ?? 0;
    if (!slug) continue; // defensive: a match always fills one group
    if (at > last) {
      segments.push({ kind: 'markdown', text: text.slice(last, at) });
    }
    segments.push({ kind: 'job', slug, url: `https://freehire.dev/jobs/${slug}` });
    last = at + m[0].length;
  }
  if (last < text.length) {
    segments.push({ kind: 'markdown', text: text.slice(last) });
  }
  // Drop markdown fragments left between/around cards that carry no prose — pure
  // whitespace, or a lone list marker (`-`, `*`, `+`, `1.`) orphaned when its
  // bullet's only content (the job link) became a card. This keeps a listed set
  // of jobs rendering as a clean stack of cards.
  const kept = segments.filter((s) => s.kind === 'job' || !isMarkerOnly(s.text));
  if (kept.length === 0) {
    kept.push({ kind: 'markdown', text });
  }
  return kept;
}

/** True for a markdown fragment that is only whitespace or a single list marker
 *  — nothing worth rendering once its job link has been unfurled into a card. */
function isMarkerOnly(text: string): boolean {
  const t = text.trim();
  return t === '' || /^([-*+]|\d+\.)$/.test(t);
}
