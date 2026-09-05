import { describe, expect, it } from 'vitest';
import { classifyLink, splitProfileLinks, mergeProfileLinks } from './profileLinks';

describe('classifyLink', () => {
  it('recognises LinkedIn member profiles, however they were pasted', () => {
    for (const url of [
      'https://www.linkedin.com/in/danaokonkwo',
      'https://linkedin.com/in/danaokonkwo',
      'https://br.linkedin.com/in/danaokonkwo',
      'linkedin.com/in/danaokonkwo',
      'https://www.linkedin.com/in/danaokonkwo/details/experience/?trk=share',
    ]) {
      expect(classifyLink(url), url).toBe('linkedin');
    }
  });

  it('recognises GitHub', () => {
    for (const url of [
      'https://github.com/octocat',
      'https://www.github.com/octocat',
      'github.com/octocat',
    ]) {
      expect(classifyLink(url), url).toBe('github');
    }
  });

  // The whole point of matching the HOST rather than searching the string: a suffix test
  // would hand an attacker-controlled domain the "this is your LinkedIn" box.
  it('rejects lookalike hosts', () => {
    for (const url of [
      'https://linkedin.com.evil.example/in/danaokonkwo',
      'https://github.com.evil.example/octocat',
      'https://notlinkedin.com/in/danaokonkwo',
      'https://evil.example/?u=https://linkedin.com/in/danaokonkwo',
    ]) {
      expect(classifyLink(url), url).toBe('other');
    }
  });

  it('treats a LinkedIn company page as other, not as a member profile', () => {
    // Only /in/<id> names a person. A company page in the "your LinkedIn" box would be
    // wrong in a way the user would not notice.
    expect(classifyLink('https://www.linkedin.com/company/northwind-systems')).toBe('other');
  });

  it('treats unparseable input as other rather than throwing', () => {
    for (const url of ['', '   ', 'not a url', 'javascript:alert(1)']) {
      expect(classifyLink(url), url).toBe('other');
    }
  });
});

describe('splitProfileLinks', () => {
  it('pulls the two named links out and keeps everything else', () => {
    const got = splitProfileLinks([
      'https://ada.example',
      'https://github.com/octocat',
      'https://www.linkedin.com/in/danaokonkwo',
    ]);

    expect(got.linkedin).toBe('https://www.linkedin.com/in/danaokonkwo');
    expect(got.github).toBe('https://github.com/octocat');
    expect(got.other).toEqual(['https://ada.example']);
  });

  it('takes the first of a repeated kind and keeps the rest as other', () => {
    // Two LinkedIn URLs is a CV that listed the same profile twice, or two accounts. Either
    // way, silently dropping one would lose data the candidate put on their CV.
    const got = splitProfileLinks([
      'https://linkedin.com/in/first',
      'https://linkedin.com/in/second',
    ]);

    expect(got.linkedin).toBe('https://linkedin.com/in/first');
    expect(got.other).toEqual(['https://linkedin.com/in/second']);
  });

  it('reports empty strings when a kind is absent', () => {
    const got = splitProfileLinks([]);
    expect(got.linkedin).toBe('');
    expect(got.github).toBe('');
    expect(got.other).toEqual([]);
  });
});

describe('mergeProfileLinks', () => {
  it('rebuilds one flat list, named links first', () => {
    expect(
      mergeProfileLinks({
        linkedin: 'https://linkedin.com/in/dana',
        github: 'https://github.com/octocat',
        other: ['https://ada.example'],
      }),
    ).toEqual([
      'https://linkedin.com/in/dana',
      'https://github.com/octocat',
      'https://ada.example',
    ]);
  });

  it('drops a field the candidate left blank', () => {
    expect(mergeProfileLinks({ linkedin: '', github: '   ', other: [] })).toEqual([]);
  });

  // The round trip is what keeps a link the classifier did not recognise from being lost
  // by a user who only edited their LinkedIn field.
  it('round-trips an unrecognised link untouched', () => {
    const original = ['https://ada.example/cv.pdf', 'https://linkedin.com/in/dana'];
    expect(mergeProfileLinks(splitProfileLinks(original)).sort()).toEqual([...original].sort());
  });
});
