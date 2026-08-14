// A source page's description arrives as sanitized HTML (internal/sources.SanitizeHTML's
// allowlist: headings, paragraphs, lists, tables, blockquote, pre/code, emphasis — see
// internal/sources/sanitize.go), but the submit form's description editor is EasyMDE,
// markdown in and out. Dropping the raw HTML straight into it renders literal `<p>`
// tags instead of formatted text. This converts the allowed subset into markdown; a tag
// outside it (there shouldn't be one, the backend already stripped it) degrades to its
// plain text rather than surfacing a raw tag.
function inlineNodes(nodes: Node[]): string {
  let out = '';
  for (const child of nodes) {
    if (child.nodeType === Node.TEXT_NODE) {
      out += child.textContent ?? '';
      continue;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue;
    const el = child as Element;
    const inner = inlineNodes(Array.from(el.childNodes));
    switch (el.tagName.toLowerCase()) {
      case 'strong':
      case 'b':
        out += `**${inner}**`;
        break;
      case 'em':
      case 'i':
        out += `*${inner}*`;
        break;
      case 'code':
        out += `\`${inner}\``;
        break;
      case 'br':
        out += '\n';
        break;
      default:
        out += inner;
    }
  }
  return out;
}

function inline(node: Node): string {
  return inlineNodes(Array.from(node.childNodes));
}

// Block-level tags SanitizeHTML allows. Anything else — bare text, `<br>`, `strong`/`em`/
// `code`/`span`/`u` sitting directly among block siblings with no wrapping `<p>` — is
// inline content and is accumulated into the paragraph being built instead of being
// dropped (a real ATS description can carry a stray text node freehire's own sanitizer
// never forces into a block).
const BLOCK_TAGS = new Set([
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'p', 'div', 'blockquote', 'pre', 'ul', 'ol', 'hr', 'table', 'dl',
]);

function blocks(node: Element): string[] {
  const out: string[] = [];
  let pending: Node[] = [];

  const flushPending = () => {
    const text = inlineNodes(pending).trim();
    if (text) out.push(text);
    pending = [];
  };

  for (const child of Array.from(node.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) {
      pending.push(child);
      continue;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue;
    const el = child as Element;
    const tag = el.tagName.toLowerCase();

    if (!BLOCK_TAGS.has(tag)) {
      pending.push(el);
      continue;
    }
    flushPending();

    const heading = /^h([1-6])$/.exec(tag);
    if (heading) {
      const text = inline(el).trim();
      if (text) out.push(`${'#'.repeat(Number(heading[1]))} ${text}`);
    } else if (tag === 'blockquote') {
      const text = inline(el).trim();
      if (text) out.push(text.split('\n').map((l) => `> ${l}`).join('\n'));
    } else if (tag === 'pre') {
      const text = (el.textContent ?? '').trim();
      if (text) out.push('```\n' + text + '\n```');
    } else if (tag === 'ul' || tag === 'ol') {
      const items = Array.from(el.children).filter((c) => c.tagName.toLowerCase() === 'li');
      const lines = items
        .map((li, i) => `${tag === 'ol' ? `${i + 1}.` : '-'} ${inline(li).trim()}`)
        .filter((l) => l.length > 2);
      if (lines.length) out.push(lines.join('\n'));
    } else if (tag === 'hr') {
      out.push('---');
    } else if (tag === 'table') {
      // GFM tables need a delimiter row after the header and leading/trailing pipes on
      // every row — a bare `cell | cell` per line (the previous approach) renders as one
      // paragraph per row, not a table.
      const rows = Array.from(el.querySelectorAll('tr')).map((tr) =>
        Array.from(tr.children).map((cell) => inline(cell).trim()),
      );
      const [header, ...body] = rows;
      if (header) {
        const width = Math.max(...rows.map((r) => r.length));
        const line = (cells: string[]) =>
          `| ${Array.from({ length: width }, (_, i) => cells[i] ?? '').join(' | ')} |`;
        out.push(
          [line(header), `| ${Array(width).fill('---').join(' | ')} |`, ...body.map(line)].join(
            '\n',
          ),
        );
      }
    } else if (tag === 'p' || tag === 'div') {
      const text = inline(el).trim();
      if (text) out.push(text);
    } else {
      // dl (or any other BLOCK_TAGS member without its own branch above): recurse for
      // nested blocks, falling back to its own inline text when it has none.
      const nested = blocks(el);
      if (nested.length) out.push(...nested);
      else {
        const text = inline(el).trim();
        if (text) out.push(text);
      }
    }
  }
  flushPending();

  return out;
}

export function htmlToMarkdown(html: string): string {
  if (!html.trim()) return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  return blocks(doc.body).join('\n\n').trim();
}
