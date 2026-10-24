import { describe, expect, it } from 'vitest';
import { demoteHeadings } from './descriptionHeadings';

describe('demoteHeadings', () => {
  it('moves the shallowest heading to h3 and keeps the rest relative', () => {
    expect(demoteHeadings('<h1>Role</h1><h2>Duties</h2><p>x</p><h3>Perks</h3>')).toBe(
      '<h3>Role</h3><h4>Duties</h4><p>x</p><h5>Perks</h5>',
    );
  });

  it('demotes a body that starts at h2', () => {
    expect(demoteHeadings('<h2>About</h2><h4>Team</h4>')).toBe('<h3>About</h3><h5>Team</h5>');
  });

  it('clamps at h6', () => {
    expect(demoteHeadings('<h1>A</h1><h5>B</h5><h6>C</h6>')).toBe('<h3>A</h3><h6>B</h6><h6>C</h6>');
  });

  it('leaves a body already at h3 or deeper untouched', () => {
    const html = '<h3>About</h3><h4>Team</h4>';
    expect(demoteHeadings(html)).toBe(html);
    expect(demoteHeadings('<p>No headings</p>')).toBe('<p>No headings</p>');
  });

  it('is idempotent', () => {
    const once = demoteHeadings('<h1>A</h1><h2>B</h2>');
    expect(demoteHeadings(once)).toBe(once);
  });

  it('handles upper case and attributes, and ignores look-alike tags', () => {
    expect(demoteHeadings('<H2 class="x">A</H2><hr><header>z</header>')).toBe(
      '<h3 class="x">A</h3><hr><header>z</header>',
    );
  });
});
