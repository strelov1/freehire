import { describe, it, expect, beforeEach } from 'vitest';
import { extractSnapshot, extractHeadline } from './scraper';

function reset() {
  document.title = '';
  document.head.replaceChildren();
  document.body.replaceChildren();
}

function addOgTitle(content: string) {
  const meta = document.createElement('meta');
  meta.setAttribute('property', 'og:title');
  meta.setAttribute('content', content);
  document.head.append(meta);
}

function addHeading(text: string) {
  const h1 = document.createElement('h1');
  h1.textContent = text;
  document.body.append(h1);
}

function addParagraph(text: string) {
  const p = document.createElement('p');
  p.textContent = text;
  document.body.append(p);
}

describe('extractHeadline', () => {
  beforeEach(reset);

  it('prefers og:title over the h1', () => {
    addOgTitle('Senior Go Engineer');
    addHeading('Some page heading');
    expect(extractHeadline(document)).toBe('Senior Go Engineer');
  });

  it('falls back to the first h1 when no og:title', () => {
    addHeading('  Backend Developer  ');
    addHeading('Ignored');
    expect(extractHeadline(document)).toBe('Backend Developer');
  });

  it('falls back to the document title when no og:title or h1', () => {
    document.title = 'Careers — Acme';
    addParagraph('no headings here');
    expect(extractHeadline(document)).toBe('Careers — Acme');
  });
});

describe('extractSnapshot', () => {
  beforeEach(reset);

  it('captures title, headline and collapsed visible text', () => {
    document.title = 'Job Page';
    addHeading('Platform Engineer');
    addParagraph('We   are\n\nhiring.');

    const snapshot = extractSnapshot(document);

    expect(snapshot.title).toBe('Job Page');
    expect(snapshot.headline).toBe('Platform Engineer');
    expect(snapshot.text).toContain('Platform Engineer');
    expect(snapshot.text).toContain('We are hiring.');
    // whitespace is collapsed, not preserved verbatim
    expect(snapshot.text).not.toContain('  ');
  });
});
