// The skill-set edits behind the job-match block's two answers to a missing skill — "I have
// it" and "I avoid it" — kept pure and apart from the store that performs them: `PUT
// /me/profile` replaces the whole profile row, so every write starts by rebuilding these two
// lists from the copy currently held.
//
// The invariant they exist to hold: a skill is never in both lists. `filtersFromProfile`
// resolves such an overlap by letting the include win and dropping the exclude SILENTLY, so a
// profile carrying one would produce a filter that ignores the viewer's avoid with nothing on
// screen to explain it. Each of the two adding functions removes the skill from the other list.
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

/** The skill sets once the viewer says they want to avoid `skill`: excluded, and no longer
 *  claimed. The mirror of `withSkill` — the same contradiction resolved from the other side. */
export function withAvoidedSkill(sets: ProfileSkillSets, skill: string): ProfileSkillSets {
  return {
    skills: sets.skills.filter((s) => !same(s, skill)),
    excluded_skills: sets.excluded_skills.some((s) => same(s, skill))
      ? sets.excluded_skills
      : [...sets.excluded_skills, skill],
  };
}

/** The skill sets once the viewer stops avoiding `skill`. It only lifts the exclusion: not
 *  wanting to avoid a skill is not the same as claiming it. */
export function withoutAvoidedSkill(sets: ProfileSkillSets, skill: string): ProfileSkillSets {
  return {
    skills: sets.skills,
    excluded_skills: sets.excluded_skills.filter((s) => !same(s, skill)),
  };
}
