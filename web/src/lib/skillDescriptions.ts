/** The skill glossary: what a canonical skill IS, in one or two sentences.
 *
 *  Generated from `internal/dict/skilltag`, the same dictionary that decides the skills
 *  themselves, so a definition cannot drift from the thing it defines.
 *
 *  The prose is imported dynamically and never re-exported eagerly. The catalog is a
 *  sentence per skill against a word per skill in `contracts.ts`, which every page loads
 *  — so reaching it through `import()` is what keeps a reader who never opens a
 *  definition from downloading one. That is the only reason anything here is async.
 *
 *  There is no "is this described?" reader any more. Every canonical carries an entry,
 *  so that question has one answer and asking it was a cost the waves imposed, not the
 *  vocabulary. */

type SkillDescriptions = Readonly<Record<string, string>>;

/** The in-flight or settled fetch, memoised. The PROMISE is memoised rather than its
 *  value: a job card renders a row of chips at once, and memoising after the await would
 *  let each of them start its own import before the first resolved. */
let catalog: Promise<SkillDescriptions> | undefined;

/** The whole catalog — for the glossary index, which needs every entry rather than one.
 *
 *  A failed import clears the memo. The chunk is fetched lazily, so the request can land
 *  after a deploy has replaced it; caching the rejection would turn one 404 into a tab
 *  that never shows a definition again. */
export function loadSkillDescriptions(): Promise<SkillDescriptions> {
  catalog ??= import('./generated/skillDescriptions')
    .then((m) => m.SKILL_DESCRIPTIONS as SkillDescriptions)
    .catch((err) => {
      catalog = undefined;
      throw err;
    });
  return catalog;
}

/** The description for one canonical slug, or `''` for a skill no wave has described
 *  yet, for a slug that is not a skill, and for a catalog that could not be fetched.
 *
 *  All three answer alike deliberately. Callers render a described skill differently
 *  from an undescribed one — no tooltip, no glossary link — so the absence has to be a
 *  value they can test. A missing definition is also not worth failing a chip over: the
 *  skill still renders, it just says nothing extra. */
export async function skillDescription(slug: string): Promise<string> {
  if (!slug) return '';
  try {
    return (await loadSkillDescriptions())[slug] ?? '';
  } catch {
    return '';
  }
}
