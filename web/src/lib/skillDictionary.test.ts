import { describe, expect, it } from 'vitest';
import { searchSkillOptions } from './skillDictionary';

const dist = [
  { value: 'solana', label: 'Solana' },
  { value: 'python', label: 'Python' },
  { value: 'go', label: 'Go' },
  { value: 'react', label: 'React' },
];

describe('searchSkillOptions', () => {
  it('finds a skill by its alias before a typo neighbour', () => {
    expect(searchSkillOptions(dist, 'golang').map((o) => o.value)[0]).toBe('go');
  });

  it('tolerates a typo', () => {
    expect(searchSkillOptions(dist, 'pyhton').map((o) => o.value)).toContain('python');
  });

  it('leaves out the skills already taken', () => {
    expect(searchSkillOptions(dist, '', ['python']).map((o) => o.value)).toEqual(['solana', 'go', 'react']);
  });

  it('offers the busiest few before anything is typed', () => {
    expect(searchSkillOptions(dist, '', [], 2).map((o) => o.value)).toEqual(['solana', 'python']);
  });
});
