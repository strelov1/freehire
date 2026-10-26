import { describe, it, expect } from 'vitest';
import { fuzzyMatch, highlightParts } from './fuzzy';

describe('fuzzyMatch', () => {
  it('matches an empty query (no filtering)', () => {
    expect(fuzzyMatch('Senior Backend Engineer', '')).toBe(true);
    expect(fuzzyMatch('Senior Backend Engineer', '   ')).toBe(true);
  });

  it('matches an exact substring, case-insensitively', () => {
    expect(fuzzyMatch('Senior Backend Engineer', 'backend')).toBe(true);
    expect(fuzzyMatch('Senior Backend Engineer', 'BACK')).toBe(true);
    expect(fuzzyMatch('Data Scientist', 'scientist')).toBe(true);
  });

  it('tolerates typos and transpositions within a word', () => {
    expect(fuzzyMatch('Senior Backend Engineer', 'sineor')).toBe(true); // transposed e/i
    expect(fuzzyMatch('Senior Backend Engineer', 'snior')).toBe(true); // dropped letter
    expect(fuzzyMatch('Senior Backend Engineer', 'seniro')).toBe(true); // transposed o/r
    expect(fuzzyMatch('Data Scientist', 'sceintist')).toBe(true); // transposition
    expect(fuzzyMatch('Backend Engineer', 'backedn')).toBe(true); // transposed d/n
    expect(fuzzyMatch('Founding Engineer', 'enginer')).toBe(true); // dropped letter
  });

  it('matches every whitespace-separated token (AND), each fuzzily', () => {
    expect(fuzzyMatch('Senior Backend Engineer', 'sineor back')).toBe(true);
    expect(fuzzyMatch('Senior Backend Engineer', 'senior enginer')).toBe(true);
  });

  it('does not match when a token has no close word', () => {
    expect(fuzzyMatch('Founding Engineer', 'xyz')).toBe(false);
    expect(fuzzyMatch('Senior Backend Engineer', 'frontend')).toBe(false);
    // one good token, one nonsense token → no match (AND semantics)
    expect(fuzzyMatch('Senior Backend Engineer', 'senior zzzzz')).toBe(false);
  });

  it('marks the closest letters of a chip label', () => {
    const marked = (label: string, query: string) =>
      highlightParts(label, query)
        .filter((part) => part.match)
        .map((part) => part.text)
        .join('');
    expect(marked('Software Engineering', 'engen')).toBe('Engin');
    expect(marked('Unreal Engine', 'engen')).toBe('Engin');
    expect(marked('Senior', 'sineor')).toBe('Senior');
    expect(marked('React Native', 'react')).toBe('React');
    expect(marked('Marketing', 'engen')).toBe('');
  });

  it('does not fuzzily collapse short distinct words', () => {
    // "lead" vs "read"/"dead" are 1 edit apart but 4-char words allow only exact-ish;
    // a real distinct role word must not match an unrelated short query.
    expect(fuzzyMatch('Product Manager', 'sales')).toBe(false);
  });
});
