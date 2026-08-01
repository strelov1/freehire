// The two skill-set edits behind claiming a skill from the job-match block, kept pure and
// apart from the store that performs them: `PUT /me/profile` replaces the whole profile
// row, so every write starts by rebuilding these two lists from the copy currently held.
//
// Case-insensitive throughout. The endpoint lowercases what it is given, so a profile
// seeded before that (or typed by hand) can hold "Docker" where a job's canonical facet
// says "docker" — comparing verbatim would let the same skill be claimed twice.

/** The two lists a claim touches. `UserProfile` satisfies this structurally, so callers
 *  hand it over directly. */
export interface ProfileSkillSets {
  skills: string[];
  excluded_skills: string[];
}

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** The skill sets once the viewer claims `skill`: held, and no longer avoided. A profile
 *  that both claims and excludes one skill is incoherent — the viewer resolved that by
 *  pressing the button, so the exclusion goes. */
export function withSkill(sets: ProfileSkillSets, skill: string): ProfileSkillSets {
  return {
    skills: sets.skills.some((s) => same(s, skill)) ? sets.skills : [...sets.skills, skill],
    excluded_skills: sets.excluded_skills.filter((s) => !same(s, skill)),
  };
}

/** The skill sets once the viewer undoes a claim. It subtracts that one skill rather than
 *  restoring a snapshot, so undoing an earlier claim leaves a later one standing. The
 *  exclusion `withSkill` dropped is not restored — that would re-create the contradiction
 *  the claim resolved. */
export function withoutSkill(sets: ProfileSkillSets, skill: string): ProfileSkillSets {
  return {
    skills: sets.skills.filter((s) => !same(s, skill)),
    excluded_skills: sets.excluded_skills,
  };
}
