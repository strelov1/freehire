import { describe, expect, it } from 'vitest';
import {
  MIN_SKILL_OPEN,
  displayAliases,
  showsPostings,
  topNeighbours,
} from './skillGlossary';

describe('displayAliases', () => {
  it('keeps the spellings a reader would not have guessed', () => {
    expect(displayAliases(['k8s', 'kubernetes'], 'kubernetes', 'Kubernetes')).toEqual(['k8s']);
  });

  // Two canonicals in three have no spelling but their own name. Rendering "also
  // written as: javascript" under a heading that says JavaScript is filler, and filler
  // is the thin-content failure the postings gate exists to prevent.
  it('is empty when the only spelling is the skill’s own name', () => {
    expect(displayAliases(['javascript'], 'javascript', 'JavaScript')).toEqual([]);
    expect(displayAliases([], 'jira', 'Jira')).toEqual([]);
  });

  it('ignores case when matching the slug and the label', () => {
    expect(displayAliases(['ABAP', 'abap'], 'abap', 'ABAP')).toEqual([]);
  });

  // The parser accepts a Latin and a Cyrillic spelling of 1C because postings are
  // written both ways. They render identically, so printing both reads as a bug.
  it('collapses spellings that differ only by a lookalike letter', () => {
    expect(displayAliases(['1c', '1с'], '1c', '1C')).toEqual([]);
    expect(displayAliases(['python', 'pуthon'], 'django', 'Django')).toEqual(['python']);
  });

  // "c++" is the label, so it goes for the same reason "javascript" does — the heading
  // above already says it. "c/c++" is the spelling a reader would not have guessed.
  it('drops the label even when the slug is spelled differently', () => {
    expect(displayAliases(['c++', 'c/c++', 'cpp'], 'cpp', 'C++')).toEqual(['c/c++']);
  });

  it('keeps the order it was given, which is the dictionary’s', () => {
    expect(displayAliases(['zeta', 'alpha'], 'x', 'X')).toEqual(['zeta', 'alpha']);
  });
});

describe('showsPostings', () => {
  // Same shape as roleLandings' gates: a block that would misdescribe the catalogue does
  // not render, and the definition above it stands on its own.
  it('renders the block only once the facet has enough postings', () => {
    expect(showsPostings(MIN_SKILL_OPEN)).toBe(true);
    expect(showsPostings(MIN_SKILL_OPEN - 1)).toBe(false);
    expect(showsPostings(0)).toBe(false);
  });
});

describe('topNeighbours', () => {
  const distribution = { kubernetes: 900, docker: 700, terraform: 700, helm: 20 };
  const allDescribed = () => true;

  it('ranks the skills that share postings with this one', () => {
    expect(topNeighbours(distribution, 'kubernetes', 2, allDescribed)).toEqual([
      'docker',
      'terraform',
    ]);
  });

  // The skill is on every posting in its own facet, so it would top its own list.
  it('drops the skill itself', () => {
    expect(topNeighbours(distribution, 'kubernetes', 10, allDescribed)).not.toContain('kubernetes');
  });

  // Every neighbour is a link, and /skills/<slug> 404s on a skill with no entry. While
  // coverage is thin an unfiltered list is a block of dead links on a page whose whole
  // claim is that it is worth linking to.
  it('drops neighbours that have no glossary page', () => {
    const described = (slug: string) => slug === 'docker';
    expect(topNeighbours(distribution, 'kubernetes', 10, described)).toEqual(['docker']);
  });

  // The limit counts what survives, so a thin patch of coverage does not silently
  // shorten the block from eight to two.
  it('fills the limit from what remains after filtering', () => {
    const described = (slug: string) => slug !== 'docker';
    expect(topNeighbours(distribution, 'kubernetes', 2, described)).toEqual([
      'terraform',
      'helm',
    ]);
  });

  it('breaks ties by slug so the page does not reshuffle between renders', () => {
    expect(topNeighbours({ b: 5, a: 5, c: 5 }, 'z', 2, allDescribed)).toEqual(['a', 'b']);
  });

  it('is empty when nothing else co-occurs', () => {
    expect(topNeighbours({ kubernetes: 900 }, 'kubernetes', 5, allDescribed)).toEqual([]);
    expect(topNeighbours({}, 'kubernetes', 5, allDescribed)).toEqual([]);
  });
});
