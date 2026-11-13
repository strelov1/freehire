// The CV-first onboarding flow's rules, kept out of the page so they can be read and tested
// on their own: which phase follows which, what each question offers, and how the answers
// become a search and a profile.
//
// THE INVARIANT: the profile saved at sign-up narrows the way the visitor's search did,
// as far as a profile can. So the search is not built from its own mapping table — it is
// the profile's own filters (`filtersFromProfile`, the same function the profile alert and
// "Apply my profile" use), minus skills, which rank the results instead of filtering them,
// plus what a profile does not hold: the time window, and the two "in your way" answers
// that narrow (visa sponsorship, remote work). The alert therefore matches the profile
// (role, level, place, skills), not those, and the copy promising it says "your profile",
// not "this search".

import { challengeChip } from './onboardingChallenge';
import { MAX_SPECIALIZATIONS } from './profileLimits';
import { filtersFromProfile, filtersToParams } from './facetModel';
import { countryLabel } from './facets';
import { categoryLabel, SENIORITY_LABELS, titleCase } from './labels';
import { regionFromCountry, WORLDWIDE_REGION } from './geoScope';
import type { LocationPreferences, OnboardingMatch, OnboardingParse, UserProfile } from './types';

export type Phase = 'drop' | 'parsing' | 'questions' | 'searching' | 'results' | 'carrying';
export type FlowEvent = 'file' | 'parsed' | 'failed' | 'answered' | 'found' | 'edit' | 'restart' | 'carry';

const TRANSITIONS: Partial<Record<Phase, Partial<Record<FlowEvent, Phase>>>> = {
  drop: { file: 'parsing' },
  parsing: { parsed: 'questions', failed: 'drop' },
  questions: { answered: 'searching' },
  searching: { found: 'results', failed: 'questions' },
  results: { edit: 'questions', carry: 'carrying' },
};

/** The phase after `event`. Starting over is allowed from anywhere; an event a phase does
 *  not take leaves it where it is. */
export function nextPhase(phase: Phase, event: FlowEvent): Phase {
  if (event === 'restart') return 'drop';
  return TRANSITIONS[phase]?.[event] ?? phase;
}

// ---- the questions ----

type WhereId = 'remote' | 'country' | 'anywhere' | 'any';

/** The three answers. Absent = not asked yet; `any` (or no challenge picked) = no
 *  preference. Where comes first because the role choices are counted under it. */
export interface Answers {
  where?: WhereId;
  /** A category code from the CV, or `any`. */
  role?: string;
  /** What is in the visitor's way (the survey's challenge vocabulary), in the order picked;
   *  empty once answered with nothing picked. */
  challenge?: string[];
  /** A note in their own words; kept only beside `other`. */
  challengeNote?: string;
}

export type QuestionKey = 'where' | 'role' | 'challenge';
export const QUESTIONS: { key: QuestionKey; chip: string }[] = [
  { key: 'where', chip: 'Where' },
  { key: 'role', chip: 'Role' },
  { key: 'challenge', chip: 'In your way' },
];

/** The first question still unanswered, or null once all three are. */
export function currentQuestion(answers: Answers): QuestionKey | null {
  return QUESTIONS.find((q) => answers[q.key] === undefined)?.key ?? null;
}

export interface Choice {
  id: string;
  label: string;
  /** Open jobs behind the choice, when the catalogue was asked. */
  count?: number;
}

const NO_PREFERENCE: Choice = { id: 'any', label: 'No preference' };

/** Remote, the CV's country (only when its location resolved to one), anywhere. */
export function whereChoices(parse: OnboardingParse): Choice[] {
  return [
    { id: 'remote', label: 'Remote' },
    ...(parse.country ? [{ id: 'country', label: countryLabel(parse.country) }] : []),
    { id: 'anywhere', label: 'Anywhere' },
    NO_PREFERENCE,
  ];
}

const MAX_ROLE_CHOICES = 4;

function seniorityLabel(s: string): string {
  return SENIORITY_LABELS[s] ?? titleCase(s);
}

function roleLabel(category: string, seniority: string | undefined): string {
  return seniority ? `${seniorityLabel(seniority)} ${categoryLabel(category)}` : categoryLabel(category);
}

/** Up to four roles, each with the CV's level when one was derived.
 *
 *  Without `counts` (the catalogue not asked yet, or unreachable) they are the CV's own
 *  categories. With them — open jobs per category for the CV's skills, from
 *  `roleFacetQuery` — the CV's categories that the catalogue holds come first (who the
 *  visitor is), then the largest other categories for their skills fill the rest (where
 *  those skills are wanted). A role with no open jobs is not offered. */
export function roleChoices(parse: OnboardingParse, counts?: Record<string, number>): Choice[] {
  const choice = (c: string) => ({ id: c, label: roleLabel(c, parse.seniority) });
  if (counts === undefined) return [...parse.categories.slice(0, MAX_ROLE_CHOICES).map(choice), NO_PREFERENCE];
  const held = Object.entries(counts).filter(([, n]) => n > 0);

  const count = new Map(held);
  const own = parse.categories.filter((c) => count.has(c));
  const others = held
    .filter(([c]) => !own.includes(c))
    .sort(([, a], [, b]) => b - a)
    .map(([c]) => c);
  const roles = [...own, ...others].slice(0, MAX_ROLE_CHOICES);
  return [...roles.map((c) => ({ ...choice(c), count: count.get(c) })), NO_PREFERENCE];
}

/** The facet query the role choices are counted under: the CV's skills (any of them) and
 *  the where answer, with no role, level or time window. */
export function roleFacetQuery(parse: OnboardingParse, answers: Answers): URLSearchParams {
  const draft = profileFromAnswers(parse, answers);
  const params = filtersToParams(
    filtersFromProfile(asProfile({ ...draft, skills: [], specializations: [], seniorities: [] })),
  );
  if (parse.skills.length > 0) params.set('skills', matchSkills(parse).join(','));
  return params;
}

/** Employers beside their logo stack: two named and what is left counted — by default
 *  the other employers, or `more` of some `noun` when the caller counts something else. */
export function companiesLine(companies: string[], more = companies.length - 2, noun = ''): string {
  const named = companies.slice(0, 2);
  if (more <= 0) return named.join(' and ');
  return `${named.join(', ')} and ${more} more${noun ? ` ${noun}` : ''}`;
}

const MAX_LOGOS = 10;

/** The employers of the top matches, each once, in rank order, at most ten. */
export function matchCompanies(matches: OnboardingMatch[]): string[] {
  return [...new Set(matches.map((m) => m.job.company).filter(Boolean))].slice(0, MAX_LOGOS);
}

/** The line beside the results' logo stack: the two leading employers, and the jobs the
 *  search found beyond the ones at those two on this page. Counted in jobs, from the
 *  search's own total — the page does not say how many employers are behind it. */
export function matchCompaniesLine(matches: OnboardingMatch[], total: number): string {
  const companies = matchCompanies(matches);
  const named = new Set(companies.slice(0, 2));
  const theirs = matches.filter((m) => named.has(m.job.company)).length;
  return companiesLine(companies, total - theirs, 'jobs');
}

/** One label per answer that says something, in question order — the progress chips and
 *  the results screen's summary. No preference says nothing and is left out. */
export function answerChips(parse: OnboardingParse, answers: Answers): string[] {
  return QUESTIONS.map((q) => answerLabel(parse, answers, q.key)).filter((l): l is string => l !== null);
}

/** The label of one answer, or null when it is unanswered or no preference. */
export function answerLabel(parse: OnboardingParse, answers: Answers, key: QuestionKey): string | null {
  switch (key) {
    case 'where':
      if (answers.where === 'country') return parse.country ? countryLabel(parse.country) : null;
      if (answers.where === 'remote') return 'Remote';
      return answers.where === 'anywhere' ? 'Anywhere' : null;
    case 'role':
      return answers.role && answers.role !== 'any' ? roleLabel(answers.role, parse.seniority) : null;
    case 'challenge':
      return challengeChip(answers.challenge ?? []);
  }
}

// ---- answers → profile and search ----

/** What `profileStore.save` takes from the flow. */
export interface ProfileDraft {
  specializations: string[];
  skills: string[];
  seniorities: string[];
  location: LocationPreferences | null;
}

/** The location block for the where answer.
 *
 *  Remote is a remote work mode, and with a country on the CV it is remote work this
 *  candidate can take from there: open worldwide, open to their region, or open to their
 *  country — the remote reach, which `filtersFromProfile` flattens to
 *  `regions=global,<region>&countries=<code>`. A country is the remote reach with no work mode — the one
 *  shape `filtersFromProfile` flattens to exactly `countries=<code>`, which is the flow's
 *  search: a base alone would be withheld (it is folded in only for physical work modes),
 *  and naming the physical modes would turn on the remote-anywhere modifier. The base is
 *  recorded too, because the CV says that is where the candidate is. */
function locationFor(where: WhereId | undefined, country: string): LocationPreferences | null {
  const none = { remote: {}, base: {}, relocation: { open: false } };
  if (where === 'remote' && country) {
    const region = regionFromCountry(country);
    const regions = region ? [WORLDWIDE_REGION, region] : [WORLDWIDE_REGION];
    return { ...none, work_modes: ['remote'], remote: { regions, countries: [country] }, base: { country } };
  }
  if (where === 'remote') return { ...none, work_modes: ['remote'] };
  if (where === 'country' && country) {
    return { ...none, remote: { countries: [country] }, base: { country } };
  }
  return null;
}

/** The profile the flow saves: the chosen role and the CV's level (every CV role and no
 *  level under no preference), the CV's skills, the where answer as location. */
export function profileFromAnswers(parse: OnboardingParse, answers: Answers): ProfileDraft {
  const anyRole = !answers.role || answers.role === 'any';
  return {
    specializations: anyRole ? parse.categories.slice(0, MAX_SPECIALIZATIONS) : [answers.role as string],
    seniorities: !anyRole && parse.seniority ? [parse.seniority] : [],
    skills: [...parse.skills],
    location: locationFor(answers.where, parse.country),
  };
}

/** The draft as a whole profile with nothing excluded — the shape `filtersFromProfile`
 *  and the profile alert read. */
export function asProfile(draft: ProfileDraft): UserProfile {
  return {
    specializations: draft.specializations,
    skills: draft.skills,
    seniorities: draft.seniorities,
    excluded_skills: [],
    excluded_sources: [],
    excluded_companies: [],
    job_languages: [],
    location_preferences: draft.location,
    derived_location: null,
    cv: null,
    created_at: null,
    updated_at: null,
  };
}

const MAX_MATCH_SKILLS = 50;

/** The skills the match search ranks by — the CV's, within the endpoint's cap. */
export function matchSkills(parse: OnboardingParse): string[] {
  return parse.skills.slice(0, MAX_MATCH_SKILLS);
}

/** The match search's query string for the answers over the last `days` days.
 *
 *  Two "in your way" answers narrow it: needing visa sponsorship keeps the postings that
 *  offer it, and wanting remote or flexible work keeps remote ones — unless the where
 *  answer already named a place, which says more than the challenge does. The others say
 *  what is hard, not what to look for, and leave the search as it is. */
export function matchQuery(parse: OnboardingParse, answers: Answers, days: number): string {
  const draft = profileFromAnswers(parse, answers);
  const params = filtersToParams(filtersFromProfile(asProfile({ ...draft, skills: [] })));
  const challenge = answers.challenge ?? [];
  if (challenge.includes('visa_sponsorship')) params.set('visa_sponsorship', 'true');
  const placed = answers.where === 'remote' || answers.where === 'country';
  if (challenge.includes('remote_flexible') && !placed) params.set('work_mode', 'remote');
  params.set('posted_within_days', String(days));
  return params.toString();
}

export const FIRST_WINDOW_DAYS = 7;
const WIDE_WINDOW_DAYS = 30;

/** The wider window to try when the first found nothing, or null to stop. */
export function widenedWindow(days: number, total: number): number | null {
  return total === 0 && days === FIRST_WINDOW_DAYS ? WIDE_WINDOW_DAYS : null;
}
