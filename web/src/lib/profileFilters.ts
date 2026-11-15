import { SENIORITY_VALUES } from './generated/contracts';
import type { UserProfile } from './types';
import {
  type JobFilters,
  appliesWorkModeRemoteAnywhere,
  emptyFacet,
  emptyFilters,
  facetAdd,
  facetSetSign,
  isPhysicalMode,
  signOf,
} from './facetModel';

// How many characters each profile list may contribute to a profile-derived query.
//
// Skills are the reason this exists: a profile may hold up to 200 skills and,
// independently, up to 200 excluded skills (userprofile.go's own `maxSkills`), each up to
// 64 characters — free text, with no dictionary check behind it. Fed straight through,
// that pair could add close to 25,000 characters, blowing well past the saved-search
// service's own length bound (internal/search/savedsearch's `maxQueryLen`, 4000) before
// the rest of the filter got a chance to matter — the toggle this seeds
// (ProfileAlertToggle) would then fail outright with a raw "query is too long" error.
//
// Excluded sources carry the SAME cap (maxExcludedCount == maxSkills), so they need a
// budget for the same reason — but a much smaller one. A source key is a crawl adapter's
// name, ~12 characters, and a person avoids a handful rather than a career's worth; 200
// characters covers a dozen and keeps the total comfortably clear of maxQueryLen.
//
// Excluded companies share that cap and get a budget sized the same way. A company slug
// (normalize.CompanySlug) runs ~10-20 characters and the list is a short blocklist of
// employers someone has ruled out, so 400 characters covers twenty-odd. The
// maximal-profile test in profileFilters.test.ts holds all four lists together under
// maxQueryLen.
const SKILL_CHAR_BUDGET = 800;

const SOURCE_CHAR_BUDGET = 200;

const COMPANY_CHAR_BUDGET = 400;

/** Values from `values`, in the profile's own order, until `maxChars` is spent.
 *
 *  Bounded by total character count rather than item count: a real pick from the skill
 *  autocomplete runs ~8 characters, so the budget comfortably covers a rich real-world
 *  profile while still capping the free-text worst case. Stops at a whole value rather
 *  than truncating one mid-string — half a source key filters on a source that does not
 *  exist. */
function charBudget(values: string[], maxChars: number): string[] {
  const out: string[] = [];
  let used = 0;
  for (const raw of values) {
    const v = raw.trim();
    if (!v) continue;
    const cost = v.length + (out.length > 0 ? 1 : 0); // +1 for the joining comma
    if (used + cost > maxChars) break;
    out.push(v);
    used += cost;
  }
  return out;
}

/** The most years of experience an entry-level-only profile should be offered, or null
 *  when any selected level is past junior — from middle up, years stated on a posting say
 *  little the level does not, and a bound would only drop real matches. Interns get one
 *  year rather than zero: "up to a year" is common wording on an internship. */
const ENTRY_EXPERIENCE_CAP: Partial<Record<string, number>> = { intern: 1, junior: 2 };

function entryExperienceCap(seniorities: string[]): number | null {
  if (seniorities.length === 0) return null;
  let cap = 0;
  for (const level of seniorities) {
    const levelCap = ENTRY_EXPERIENCE_CAP[level];
    if (levelCap == null) return null;
    cap = Math.max(cap, levelCap);
  }
  return cap;
}

/** A facet that includes each of `values`. */
const seed = (values: string[]) => values.reduce(facetAdd, emptyFacet());

/** Build a fresh filter set seeded from a user profile — the reset-and-seed behind
 *  "Apply my profile". Specializations become `category` values, skills become included
 *  `skills` values, and excluded skills become EXCLUDED `skills` values (rendering
 *  `skills_exclude=…` → `skills != "X"`). A skill that is somehow both wanted and avoided
 *  stays wanted (include wins), mirroring the server's overlap rule so the two never
 *  self-cancel. The optional location block flattens into the location facets: work_modes → `work_mode`;
 *  regions from the remote reach ∪ relocation targets; countries from the remote reach ∪
 *  base ∪ relocation targets; cities from the base ∪ relocation targets; and `relocation`
 *  staged as supported+required when the user is open to relocating. The flatten is lossy
 *  (base vs relocation merge) — the filter is a convenience narrowing of "places relevant to
 *  me". Trimming/dedup come free from facetAdd, so unions of overlapping lists are safe.
 *  Avoided SOURCES and COMPANIES become excluded `source` and `company_slug` values; unlike
 *  skills they have no wanted counterpart, so there is no overlap rule to apply. Every list
 *  is bounded by `charBudget` before seeding — see its own comment. */
export function filtersFromProfile(profile: UserProfile): JobFilters {
  const f = emptyFilters();
  f.facets.category = seed(profile.specializations);
  f.facets.posting_language = seed(profile.job_languages ?? []);
  // The selected levels are included, together with the postings that carry no level at
  // all — most of the catalogue, which an include alone would drop. Including rather than
  // excluding the rest is what makes "Apply my profile" show the levels the user picked
  // (freehire#3397: an intern saw seven red excludes and an unticked Intern, and read it as
  // the profile not applying). An empty list, or every level, imposes no restriction.
  const levels = profile.seniorities ?? [];
  if (levels.length && !SENIORITY_VALUES.every((v) => levels.includes(v))) {
    f.facets.seniority = seed(SENIORITY_VALUES.filter((v) => levels.includes(v)));
    f.seniorityIncludeUnspecified = true;
    // The level include cannot judge a posting with no level, and for someone after an
    // entry role that is most of what it lets through — an intern's alerts were 19 of 19
    // unlabelled, ten of them asking for 2-5 years (freehire#3397). The posting's stated
    // years are the backstop for THOSE postings only: one labelled with a picked level
    // passes whatever it states, since most internships state no years at all (95,645
    // labelled intern, 4,071 of them stating at most a year — prod, 2026-10-26). An
    // unlabelled posting must state a bound within the cap; keeping the unstated ones too
    // was what flooded the alert with 2.6M postings (freehire#3655).
    const cap = entryExperienceCap(profile.seniorities);
    if (cap != null) {
      f.experienceYearsMax = cap;
      f.experienceOnlySeniorityUnspecified = true;
    }
  }
  // Skills: wanted → include, avoided → exclude. Only stage an exclude for a token not
  // already wanted (signOf === 'off'), so a stray overlap keeps the wanted value.
  f.facets.skills = charBudget(profile.excluded_skills ?? [], SKILL_CHAR_BUDGET).reduce(
    (st, raw) => {
      const v = raw.trim();
      return v && signOf(st, v) === 'off' ? facetSetSign(st, v, 'exclude') : st;
    },
    seed(charBudget(profile.skills, SKILL_CHAR_BUDGET)),
  );

  // Sources and companies: avoided → excluded. Neither has a wanted counterpart, so there is
  // nothing to reconcile — every value goes in as an exclude. Until each list was seeded
  // here it was stored and editable in the profile's Avoid card, yet read by nobody: a
  // control that changed no result anywhere.
  const seedExcluded = (values: string[], maxChars: number) =>
    charBudget(values, maxChars).reduce((st, v) => facetSetSign(st, v, 'exclude'), emptyFacet());
  f.facets.source = seedExcluded(profile.excluded_sources ?? [], SOURCE_CHAR_BUDGET);
  f.facets.company_slug = seedExcluded(profile.excluded_companies ?? [], COMPANY_CHAR_BUDGET);

  const loc = profile.location_preferences;
  if (loc) {
    // Relocation targets only count when the user is actually open to relocating — `open`
    // gates the whole relocation contribution (targets and the relocation facet alike).
    const reloc = loc.relocation.open ? loc.relocation : { regions: [], countries: [], cities: [] };
    // `base` says where the user LIVES; the facets say where they want the WORK. Those
    // coincide only for someone who accepts physical work — the job has to be commutable
    // — so base is folded in for them and withheld from everyone else. Seeding a
    // remote-only candidate's home country as a job-country filter would narrow their
    // search to the one country they least need the job to be in.
    //
    // This gate used to be implicit: the profile form collected `base` from on-site and
    // hybrid users only, and dropped it on save for everyone else. Now that the form asks
    // every user where they are, the gate has to be stated here instead.
    const wantsPhysical = (loc.work_modes ?? []).some(isPhysicalMode);
    const base = wantsPhysical ? loc.base : {};
    const reachRegions = loc.remote.regions ?? [];
    const reachCountries = loc.remote.countries ?? [];
    f.facets.work_mode = seed(loc.work_modes ?? []);
    // Where physical work may be: home and the relocation targets.
    f.facets.regions = seed([...(reloc.regions ?? [])]);
    f.facets.countries = seed([...(base.country ? [base.country] : []), ...(reloc.countries ?? [])]);
    f.facets.cities = seed([...(base.city ? [base.city] : []), ...(reloc.cities ?? [])]);
    // A profile accepting remote AND a physical format AND stating where the physical
    // work may be cannot survive a plain AND of modes and geography: that narrows
    // remote work to the home geography, or widens physical work to the remote reach.
    // Stage the server's remote-anywhere modifier instead — remote within the reach
    // (anywhere, when none is stated), physical within the location facets above.
    if ((loc.work_modes ?? []).includes('remote') && appliesWorkModeRemoteAnywhere(f)) {
      f.workModeRemoteAnywhere = true;
      f.workModeRemoteRegions = reachRegions;
      // A stated reach says how far remote work may come from, not that home is excluded:
      // `regions=global` matches only postings labelled worldwide, so without this a
      // remote job restricted to the user's own country would disappear. No reach stays
      // unbounded, which covers home already.
      const reaches = reachRegions.length > 0 || reachCountries.length > 0;
      f.workModeRemoteCountries =
        reaches && base.country && !reachCountries.includes(base.country)
          ? [...reachCountries, base.country]
          : reachCountries;
      // The relocation facet is not a location facet: the server ANDs it around the
      // whole disjunction, so remote postings would have to offer relocation too.
      // The relocation targets already bound the physical arm's geography.
    } else {
      // Every other shape keeps the plain flatten: one AND of the modes with the union
      // of the remote reach and the physical geography.
      f.facets.regions = seed([...reachRegions, ...(reloc.regions ?? [])]);
      f.facets.countries = seed([
        ...reachCountries,
        ...(base.country ? [base.country] : []),
        ...(reloc.countries ?? []),
      ]);
      if (loc.relocation.open) f.facets.relocation = seed(['supported', 'required']);
    }
  }
  return f;
}
