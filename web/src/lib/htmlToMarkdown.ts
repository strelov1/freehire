// A source page's description arrives as sanitized HTML (internal/sources.SanitizeHTML's
// allowlist: headings, paragraphs, lists, tables, blockquote, pre/code, emphasis — see
// internal/sources/sanitize.go), but the submit form's description editor is EasyMDE,
// markdown in and out. Dropping the raw HTML straight into it renders literal `<p>`
// tags instead of formatted text. This converts the allowed subset into markdown; a tag
// outside it (there shouldn't be one, the backend already stripped it) degrades to its
// plain text rather than surfacing a raw tag.
function inline(node: Node): string {
  let out = '';
  for (const child of Array.from(node.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) {
      out += child.textContent ?? '';
      continue;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue;
    const el = child as Element;
    const inner = inline(el);
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

function blocks(node: Element): string[] {
  const out: string[] = [];
  for (const child of Array.from(node.children)) {
    const tag = child.tagName.toLowerCase();
    const heading = /^h([1-6])$/.exec(tag);
    if (heading) {
      const text = inline(child).trim();
      if (text) out.push(`${'#'.repeat(Number(heading[1]))} ${text}`);
    } else if (tag === 'blockquote') {
      const text = inline(child).trim();
      if (text) out.push(text.split('\n').map((l) => `> ${l}`).join('\n'));
    } else if (tag === 'pre') {
      const text = (child.textContent ?? '').trim();
      if (text) out.push('```\n' + text + '\n```');
    } else if (tag === 'ul' || tag === 'ol') {
      const items = Array.from(child.children).filter((c) => c.tagName.toLowerCase() === 'li');
      const lines = items
        .map((li, i) => `${tag === 'ol' ? `${i + 1}.` : '-'} ${inline(li).trim()}`)
        .filter((l) => l.length > 2);
      if (lines.length) out.push(lines.join('\n'));
    } else if (tag === 'hr') {
      out.push('---');
    } else if (tag === 'table') {
      const rows = Array.from(child.querySelectorAll('tr')).map((tr) =>
        Array.from(tr.children)
          .map((cell) => inline(cell).trim())
          .join(' | '),
      );
      if (rows.length) out.push(rows.join('\n'));
    } else if (tag === 'p' || tag === 'div' || tag === 'span') {
      const text = inline(child).trim();
      if (text) out.push(text);
    } else {
      // An unrecognized structural wrapper (e.g. dl/dt/dd): recurse for nested blocks,
      // falling back to its own inline text when it has none.
      const nested = blocks(child);
      if (nested.length) out.push(...nested);
      else {
        const text = inline(child).trim();
        if (text) out.push(text);
      }
    }
  }
  return out;
}

export function htmlToMarkdown(html: string): string {
  if (!html.trim()) return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  return blocks(doc.body).join('\n\n').trim();
}
