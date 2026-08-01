import { describe, it, expect } from 'vitest';
import {
  withSkill,
  withoutSkill,
  withAvoidedSkill,
  withoutAvoidedSkill,
} from './profileSkills';

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

describe('withAvoidedSkill', () => {
  it('records the skill as one to avoid', () => {
    expect(withAvoidedSkill({ skills: ['docker'], excluded_skills: [] }, 'wordpress')).toEqual({
      skills: ['docker'],
      excluded_skills: ['wordpress'],
    });
  });

  it('adds nothing when the skill is already avoided, whatever its case', () => {
    expect(withAvoidedSkill({ skills: [], excluded_skills: ['WordPress'] }, 'wordpress')).toEqual({
      skills: [],
      excluded_skills: ['WordPress'],
    });
  });

  it('stops claiming a skill the viewer now avoids — never both lists at once', () => {
    expect(withAvoidedSkill({ skills: ['PHP', 'go'], excluded_skills: [] }, 'php')).toEqual({
      skills: ['go'],
      excluded_skills: ['php'],
    });
  });

  it('does not mutate what it was given', () => {
    const before = { skills: ['php'], excluded_skills: [] };
    withAvoidedSkill(before, 'php');
    expect(before).toEqual({ skills: ['php'], excluded_skills: [] });
  });
});

describe('withoutAvoidedSkill', () => {
  it('lifts the avoid, leaving another one standing', () => {
    expect(
      withoutAvoidedSkill({ skills: ['go'], excluded_skills: ['php', 'wordpress'] }, 'php'),
    ).toEqual({ skills: ['go'], excluded_skills: ['wordpress'] });
  });

  it('matches the avoided skill whatever its case', () => {
    expect(withoutAvoidedSkill({ skills: [], excluded_skills: ['PHP'] }, 'php')).toEqual({
      skills: [],
      excluded_skills: [],
    });
  });

  it('leaves the held skills alone — lifting an avoid does not claim the skill', () => {
    expect(withoutAvoidedSkill({ skills: ['go'], excluded_skills: ['php'] }, 'php')).toEqual({
      skills: ['go'],
      excluded_skills: [],
    });
  });

  it('does not mutate what it was given', () => {
    const before = { skills: [], excluded_skills: ['php'] };
    withoutAvoidedSkill(before, 'php');
    expect(before).toEqual({ skills: [], excluded_skills: ['php'] });
  });
});
