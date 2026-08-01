import { describe, it, expect } from 'vitest';
import { withSkill, withoutSkill } from './profileSkills';

describe('withSkill', () => {
  it('appends the claimed skill', () => {
    expect(withSkill({ skills: ['docker'], excluded_skills: [] }, 'bash')).toEqual({
      skills: ['docker', 'bash'],
      excluded_skills: [],
    });
  });

  it('adds nothing when the skill is already held, whatever its case', () => {
    expect(withSkill({ skills: ['Docker'], excluded_skills: [] }, 'docker')).toEqual({
      skills: ['Docker'],
      excluded_skills: [],
    });
  });

  it('stops excluding a skill the viewer just claimed', () => {
    expect(withSkill({ skills: ['docker'], excluded_skills: ['PHP', 'perl'] }, 'php')).toEqual({
      skills: ['docker', 'php'],
      excluded_skills: ['perl'],
    });
  });

  it('does not mutate what it was given', () => {
    const before = { skills: ['docker'], excluded_skills: ['bash'] };
    withSkill(before, 'bash');
    expect(before).toEqual({ skills: ['docker'], excluded_skills: ['bash'] });
  });
});

describe('withoutSkill', () => {
  it('removes only the named skill, keeping one claimed after it', () => {
    expect(
      withoutSkill({ skills: ['docker', 'bash', 'powershell'], excluded_skills: [] }, 'bash'),
    ).toEqual({ skills: ['docker', 'powershell'], excluded_skills: [] });
  });

  it('matches the held skill whatever its case', () => {
    expect(withoutSkill({ skills: ['Bash'], excluded_skills: [] }, 'bash')).toEqual({
      skills: [],
      excluded_skills: [],
    });
  });

  it('leaves the excluded set alone — undoing a claim does not re-exclude the skill', () => {
    expect(withoutSkill({ skills: ['php'], excluded_skills: ['perl'] }, 'php')).toEqual({
      skills: [],
      excluded_skills: ['perl'],
    });
  });

  it('does not mutate what it was given', () => {
    const before = { skills: ['docker', 'bash'], excluded_skills: [] };
    withoutSkill(before, 'bash');
    expect(before).toEqual({ skills: ['docker', 'bash'], excluded_skills: [] });
  });
});
