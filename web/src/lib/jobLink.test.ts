import { describe, it, expect } from 'vitest';
import { linkInText, pastedJobLink } from './jobLink';

const ORIGIN = 'https://freehire.me';

describe('linkInText', () => {
  it('takes a URL that says so itself', () => {
    expect(linkInText('https://job-boards.greenhouse.io/acme/jobs/123')).toBe(
      'https://job-boards.greenhouse.io/acme/jobs/123',
    );
    expect(linkInText('http://acme.com/careers/1')).toBe('http://acme.com/careers/1');
  });

  // Copying from the address bar drops the scheme, so this is what a paste usually is.
  it('takes a bare host with a path, and puts a scheme back on it', () => {
    expect(linkInText('job-boards.greenhouse.io/acme/jobs/123')).toBe(
      'https://job-boards.greenhouse.io/acme/jobs/123',
    );
    expect(linkInText('acme.com/jobs/1?src=x#top')).toBe('https://acme.com/jobs/1?src=x#top');
  });

  it('ignores the surrounding whitespace a paste brings with it', () => {
    expect(linkInText('  https://acme.com/jobs/1  ')).toBe('https://acme.com/jobs/1');
  });

  // Every one of these is a query somebody typed on purpose. Reading one as a link would
  // replace the whole dropdown with an offer to import it.
  it.each([
    'go',
    'senior go developer',
    'node.js',
    'react.dev',
    'express.js',
    'c++',
    'c++/cli',
    'front end',
    'https://',
    '1.5/10',
    '',
    '   ',
  ])('leaves %o to the search', (text) => {
    expect(linkInText(text)).toBeNull();
  });

  it('leaves a scheme we would not fetch alone', () => {
    expect(linkInText('mailto:jobs@acme.com')).toBeNull();
    expect(linkInText('ftp://acme.com/jobs/1')).toBeNull();
  });

  // A domain with nothing after it is a COMPANY, and searching for the company is the
  // better of the two answers.
  it('leaves a bare domain to the search', () => {
    expect(linkInText('greenhouse.io')).toBeNull();
    expect(linkInText('acme.com')).toBeNull();
  });
});

describe('pastedJobLink', () => {
  it('reports nothing for a query', () => {
    expect(pastedJobLink('senior go developer', ORIGIN)).toBeNull();
  });

  it('reports an outside link with no slug of ours', () => {
    expect(pastedJobLink('https://acme.com/jobs/1', ORIGIN)).toEqual({
      url: 'https://acme.com/jobs/1',
      ownSlug: null,
    });
  });

  // Pasting one of our own links back in is a thing people do, and there is nothing to
  // look up: the slug is in the path.
  it('reads the slug out of one of our own posting links', () => {
    expect(pastedJobLink('https://freehire.me/jobs/go-dev-acme', ORIGIN)?.ownSlug).toBe(
      'go-dev-acme',
    );
  });

  it('judges "ours" by the running origin, so it holds on localhost too', () => {
    expect(pastedJobLink('http://localhost:5173/jobs/go-dev-acme', 'http://localhost:5173')?.ownSlug)
      .toBe('go-dev-acme');
    // The same link is somebody else's site when served from production.
    expect(pastedJobLink('http://localhost:5173/jobs/go-dev-acme', ORIGIN)?.ownSlug).toBeNull();
  });

  it('decodes a slug the path escaped', () => {
    expect(pastedJobLink('https://freehire.me/jobs/c%2B%2B-dev', ORIGIN)?.ownSlug).toBe('c++-dev');
  });

  // A lone `%` is a legal path and an illegal escape. This runs in a $derived on every
  // keystroke in the header, so a throw here would take the page down, not the row.
  it('survives a path that cannot be decoded', () => {
    expect(() => pastedJobLink('https://freehire.me/jobs/100%', ORIGIN)).not.toThrow();
    expect(pastedJobLink('https://freehire.me/jobs/100%', ORIGIN)?.ownSlug).toBe('100%');
  });

  // Our own pages that are not postings take the ordinary path: the intake finding them
  // uninteresting is the honest answer, and guessing a slug from them would not be.
  it.each([
    'https://freehire.me/companies/acme',
    'https://freehire.me/jobs',
    'https://freehire.me/jobs/go-dev-acme/apply',
  ])('claims no slug for %o', (url) => {
    expect(pastedJobLink(url, ORIGIN)?.ownSlug).toBeNull();
  });
});
