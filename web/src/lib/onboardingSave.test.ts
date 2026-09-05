import { describe, expect, it, vi } from 'vitest';
import { effectiveTotalYears, persistStep, type SaveDeps, type WizardAnswers } from './onboardingSave';
import { splitProfileLinks } from './profileLinks';
import type { CandidateContacts } from './types';

/** An account that has edited its profile through /my/profile before ever seeing the
 *  wizard. Everything here must survive every step. */
const existingContacts: CandidateContacts = {
  full_name: 'Dana Okonkwo',
  email: 'dana@example.com',
  headline: 'Staff engineer, distributed systems',
  summary: 'Twelve years of building things that stay up.',
  languages: ['English', 'Igbo'],
  certifications: ['CKA'],
  links: ['https://dana.example'],
};

/** The one contacts body the step under test sent. Fails loudly rather than reading
 *  `undefined` off an empty array, so "wrote nothing" cannot pass as "wrote the right
 *  thing". */
function onlyContactsBody(bodies: CandidateContacts[]): CandidateContacts {
  expect(bodies).toHaveLength(1);
  const body = bodies[0];
  if (body === undefined) throw new Error('no contacts body was sent');
  return body;
}

function answers(over: Partial<WizardAnswers> = {}): WizardAnswers {
  return {
    specializations: [],
    skills: [],
    seniorities: [],
    excludedSkills: [],
    location: null,
    links: { linkedin: '', github: '', other: [] },
    contacts: {},
    totalYears: null,
    derivedTotalYears: null,
    currentIncome: null,
    desiredSalary: null,
    currency: 'USD',
    period: 'month',
    stage: null,
    challenge: null,
    challengeNote: '',
    ...over,
  };
}

function deps(): SaveDeps & {
  contactsBodies: CandidateContacts[];
  screeningPatches: unknown[];
  surveyPatches: unknown[];
  profileCalls: number;
} {
  const contactsBodies: CandidateContacts[] = [];
  const screeningPatches: unknown[] = [];
  const surveyPatches: unknown[] = [];
  let profileCalls = 0;
  return {
    contactsBodies,
    screeningPatches,
    surveyPatches,
    get profileCalls() {
      return profileCalls;
    },
    saveProfile: vi.fn(async () => {
      profileCalls += 1;
    }),
    // Mirrors the server: whatever body arrives IS the new stored overlay.
    putResumeContacts: vi.fn(async (c: CandidateContacts) => {
      contactsBodies.push(c);
      return c;
    }),
    updateScreeningAnswers: vi.fn(async (p) => {
      screeningPatches.push(p);
    }),
    updateSurvey: vi.fn(async (p) => {
      surveyPatches.push(p);
    }),
  };
}

// PUT /me/resume/contacts is a FULL REPLACE — the handler marshals the body over
// users.candidate_contacts wholesale. A partial body does not update two fields, it deletes
// every other one.
describe('the contacts overlay survives every write', () => {
  it('carries the account’s existing overlay into the links write', async () => {
    const d = deps();
    // Built the way the page builds it: the stored list is split, the candidate fills the
    // LinkedIn box, and the link the classifier did not recognise rides along in `other`.
    const links = { ...splitProfileLinks(existingContacts.links ?? []), linkedin: 'https://linkedin.com/in/dana' };
    await persistStep('confirm', answers({ contacts: existingContacts, links }), d);

    const body = onlyContactsBody(d.contactsBodies);
    expect(body.headline).toBe(existingContacts.headline);
    expect(body.summary).toBe(existingContacts.summary);
    expect(body.languages).toEqual(existingContacts.languages);
    expect(body.links).toEqual(['https://linkedin.com/in/dana', 'https://dana.example']);
  });

  it('carries it into the years write too', async () => {
    const d = deps();
    await persistStep('experience', answers({ contacts: existingContacts, totalYears: 12 }), d);

    const body = onlyContactsBody(d.contactsBodies);
    expect(body.headline).toBe(existingContacts.headline);
    expect(body.total_years).toBe(12);
    expect(body.total_years_set).toBe(true);
  });

  // The regression that made this module worth extracting: `confirm` runs two steps before
  // `experience`, so a stale spread would silently drop the links the earlier step wrote.
  it('does not let a later step drop what an earlier one wrote', async () => {
    const d = deps();
    let contacts = existingContacts;

    contacts = await persistStep(
      'confirm',
      answers({ contacts, links: { linkedin: 'https://linkedin.com/in/dana', github: '', other: [] } }),
      d,
    );
    contacts = await persistStep('experience', answers({ contacts, totalYears: 12 }), d);

    expect(contacts.links).toContain('https://linkedin.com/in/dana');
    expect(contacts.total_years).toBe(12);
    expect(contacts.headline).toBe(existingContacts.headline);
  });

  it('writes nothing at all when there are no links to write', async () => {
    const d = deps();
    const out = await persistStep('confirm', answers({ contacts: existingContacts }), d);

    expect(d.contactsBodies).toHaveLength(0);
    expect(out).toBe(existingContacts);
  });
});

describe('years of experience', () => {
  // Agreeing with a pre-filled figure IS answering the question. The wizard runs once per
  // account, so treating a passed-through pre-fill as silence loses it for good.
  it('records the CV’s figure when the candidate passes through without touching it', async () => {
    const d = deps();
    await persistStep('experience', answers({ derivedTotalYears: 7 }), d);

    expect(onlyContactsBody(d.contactsBodies).total_years).toBe(7);
  });

  it('prefers what the candidate set over what the CV computed', () => {
    expect(effectiveTotalYears(answers({ totalYears: 3, derivedTotalYears: 7 }))).toBe(3);
  });

  it('records an explicit zero, which is a real answer', async () => {
    const d = deps();
    await persistStep('experience', answers({ totalYears: 0, derivedTotalYears: 7 }), d);

    const body = onlyContactsBody(d.contactsBodies);
    expect(body.total_years).toBe(0);
    expect(body.total_years_set).toBe(true);
  });

  it('writes nothing when there is neither a set value nor a CV figure', async () => {
    const d = deps();
    await persistStep('experience', answers(), d);

    expect(d.contactsBodies).toHaveLength(0);
  });
});

describe('money', () => {
  it('sends the desired salary to the screening answers and the income to the survey', async () => {
    const d = deps();
    await persistStep('money', answers({ desiredSalary: 8000, currentIncome: 5000 }), d);

    expect(d.screeningPatches).toEqual([
      { desired_salary_amount: 8000, desired_salary_currency: 'USD', desired_salary_period: 'month' },
    ]);
    expect(d.surveyPatches).toEqual([
      { current_income_amount: 5000, current_income_currency: 'USD', current_income_period: 'month' },
    ]);
  });

  // A slider dragged right and back sets 0, which BOTH endpoints reject outright. Sending it
  // would leave the candidate stuck on an error no retry can clear, with Skip — which throws
  // the answer away — as the only exit.
  it('treats a zeroed slider as no answer rather than sending a value the server rejects', async () => {
    const d = deps();
    await persistStep('money', answers({ desiredSalary: 0, currentIncome: 0 }), d);

    expect(d.screeningPatches).toHaveLength(0);
    expect(d.surveyPatches).toHaveLength(0);
  });

  it('sends only the half the candidate answered', async () => {
    const d = deps();
    await persistStep('money', answers({ desiredSalary: 8000 }), d);

    expect(d.screeningPatches).toHaveLength(1);
    expect(d.surveyPatches).toHaveLength(0);
  });
});

describe('the survey steps', () => {
  it('sends a stage', async () => {
    const d = deps();
    await persistStep('stage', answers({ stage: 'searching' }), d);
    expect(d.surveyPatches).toEqual([{ job_search_stage: 'searching' }]);
  });

  it('sends a note only alongside "other"', async () => {
    const d = deps();
    await persistStep('challenge', answers({ challenge: 'other', challengeNote: ' visas ' }), d);
    expect(d.surveyPatches).toEqual([{ biggest_challenge: 'other', biggest_challenge_note: 'visas' }]);
  });

  // The server rejects a note beside a coded challenge, so sending a leftover one would turn
  // a valid answer into a 400.
  it('drops a leftover note when the challenge is a coded one', async () => {
    const d = deps();
    await persistStep('challenge', answers({ challenge: 'english', challengeNote: 'visas' }), d);
    expect(d.surveyPatches).toEqual([{ biggest_challenge: 'english' }]);
  });

  it('writes nothing for an unanswered step', async () => {
    const d = deps();
    await persistStep('stage', answers(), d);
    await persistStep('challenge', answers(), d);
    expect(d.surveyPatches).toHaveLength(0);
  });
});

describe('the profile steps', () => {
  it('saves once both a specialization and a skill exist', async () => {
    const d = deps();
    await persistStep('skills', answers({ specializations: ['backend'], skills: ['go'] }), d);
    expect(d.profileCalls).toBe(1);
  });

  // The endpoint rejects either being empty, so there is nowhere for a level-only save to
  // land. Skipping beats failing: blocking here would make an optional step mandatory.
  it('skips the save when the profile could not exist yet', async () => {
    const d = deps();
    await persistStep('location', answers({ seniorities: ['senior'] }), d);
    expect(d.profileCalls).toBe(0);
  });
});

describe('the CV step', () => {
  it('writes nothing of its own — the upload and the import persist as they run', async () => {
    const d = deps();
    await persistStep('cv', answers({ contacts: existingContacts }), d);

    expect(d.contactsBodies).toHaveLength(0);
    expect(d.profileCalls).toBe(0);
    expect(d.surveyPatches).toHaveLength(0);
  });
});
