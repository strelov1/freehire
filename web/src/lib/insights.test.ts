import { describe, it, expect } from 'vitest';
import type { InsightRole, InsightSalaryBand, InsightSkill } from './api';
import {
  coveredCategories,
  isCovered,
  sortBandsBySeniority,
  formatSalary,
  salaryIntro,
  skillsIntro,
  rolesIntro,
  MIN_CATEGORY_OPEN,
  seniorityLabel,
  rankedQualifyingRoles,
  roleAddressExists,
  roleQualifies,
  roleSkillsIntro,
} from './insights';

const role = (category: string, seniority: string, open_count: number): InsightRole => ({
  category,
  seniority,
  open_count,
  growth: 0,
});

describe('coveredCategories', () => {
  it('publishes only categories whose total open-count clears the floor', () => {
    const roles = [
      role('backend', 'senior', MIN_CATEGORY_OPEN),
      role('backend', 'junior', 5),
      role('mobile', 'senior', MIN_CATEGORY_OPEN - 1), // below floor → excluded
    ];
    const covered = coveredCategories(roles).map((c) => c.category);
    expect(covered).toContain('backend');
    expect(covered).not.toContain('mobile');
  });

  it("excludes 'other' and blank categories", () => {
    const roles = [role('other', 'senior', 1000), role('', 'senior', 1000)];
    expect(coveredCategories(roles)).toHaveLength(0);
  });

  it('sorts by demand descending', () => {
    const roles = [
      role('frontend', 'senior', 100),
      role('backend', 'senior', 300),
      role('design', 'senior', 200),
    ];
    expect(coveredCategories(roles).map((c) => c.category)).toEqual(['backend', 'design', 'frontend']);
  });

  it('isCovered mirrors the gate', () => {
    const roles = [role('backend', 'senior', 1000)];
    expect(isCovered(roles, 'backend')).toBe(true);
    expect(isCovered(roles, 'mobile')).toBe(false);
  });
});

describe('sortBandsBySeniority', () => {
  it('orders by seniority rank with the category-wide band last', () => {
    const band = (seniority: string): InsightSalaryBand => ({
      seniority,
      currency: 'USD',
      period: 'year',
      sample_size: 10,
      p25: 1,
      p50: 2,
      p75: 3,
    });
    const sorted = sortBandsBySeniority([band('senior'), band(''), band('junior')]).map((b) => b.seniority);
    expect(sorted).toEqual(['junior', 'senior', '']);
  });
});

describe('formatSalary', () => {
  it('formats a known currency as currency', () => {
    expect(formatSalary(155000, 'USD')).toBe('$155,000');
  });
  it('normalizes a lowercase ISO code (Intl is case-insensitive)', () => {
    expect(formatSalary(1000, 'usd')).toBe('$1,000');
  });
  it('falls back gracefully for a malformed currency code', () => {
    expect(formatSalary(1000, 'US')).toBe('1,000 US');
  });
});

describe('auto-intros', () => {
  it('salaryIntro states the median from the richest yearly band', () => {
    const bands: InsightSalaryBand[] = [
      { seniority: '', currency: 'USD', period: 'year', sample_size: 200, p25: 130000, p50: 155000, p75: 180000 },
    ];
    const intro = salaryIntro('backend', bands);
    expect(intro).toContain('Backend');
    expect(intro).toContain('$155,000');
    expect(intro).toContain('200 postings');
  });

  it('salaryIntro degrades when there is no yearly band', () => {
    expect(salaryIntro('backend', [])).toContain('Backend');
  });

  it('skillsIntro names the top skills', () => {
    const skills: InsightSkill[] = [
      { skill: 'go', open_count: 100, growth: 5 },
      { skill: 'sql', open_count: 60, growth: 2 },
      { skill: 'kubernetes', open_count: 40, growth: 1 },
    ];
    expect(skillsIntro('backend', skills)).toContain('go, sql, kubernetes');
  });

  it('rolesIntro totals open roles', () => {
    const roles = [role('backend', 'senior', 30), role('backend', 'junior', 20)];
    expect(rolesIntro('backend', roles)).toContain('50 open Backend roles');
  });
});

describe('seniorityLabel', () => {
  // The category-wide band is an /insights concept, not a value of the seniority
  // vocabulary — it must keep its label here even though every real token is now
  // resolved through the shared map.
  it('names the category-wide band', () => {
    expect(seniorityLabel('')).toBe('All levels');
  });

  it('resolves a real seniority token through the shared vocabulary', () => {
    expect(seniorityLabel('c_level')).toBe('C-level');
    expect(seniorityLabel('senior')).toBe('Senior');
  });
});

describe('roleQualifies / rankedQualifyingRoles', () => {
  it('lists in the SITEMAP only a real seniority in a covered category with enough demand', () => {
    const roles = [
      role('backend', 'senior', MIN_CATEGORY_OPEN),
      // Same covered category, but this level alone is too thin for its own page.
      role('backend', 'junior', MIN_CATEGORY_OPEN - 1),
      // The category-wide band is not a seniority, so it never gets a leaf.
      role('backend', '', MIN_CATEGORY_OPEN),
      // A category that does not clear the gate takes its levels with it.
      role('qa', 'lead', MIN_CATEGORY_OPEN - 1),
    ];

    expect(rankedQualifyingRoles(roles).map((r) => [r.category, r.seniority])).toEqual([
      ['backend', 'senior'],
    ]);

    // But the thin one is still a real ADDRESS, so the route serves it (noindex) rather
    // than refusing it — the job page links there without knowing the role's size.
    expect(roleAddressExists('backend', 'junior')).toBe(true);
    // And so is a role in a category too small to be COVERED. The job page links from a
    // posting knowing only its two facets, so a coverage requirement here would send a
    // real posting's real role to a 404.
    expect(roleAddressExists('qa', 'lead')).toBe(true);
  });

  it('asks about the role handed to it, not about its rank in the list', () => {
    // The gate list is capped at 200 by the endpoint while production carries ~349 roles
    // over the floor. A role absent from the ranking must still qualify on its own
    // numbers — reading qualification off the list is what 404'd 149 real pages.
    const ranking = [role('backend', 'senior', 5000)];

    expect(roleQualifies(ranking, 'backend', 'middle', MIN_CATEGORY_OPEN)).toBe(true);
    expect(rankedQualifyingRoles(ranking).map((r) => r.seniority)).toEqual(['senior']);
  });

  it('answers whether an address exists without needing the role\'s size', () => {
    // Takes no ranking on purpose — the job page links here knowing only a posting's two
    // facets. It must also be answerable BEFORE the API is asked: the endpoint answers an
    // invented level with a 400 and a load turns that into a 500, so a mistyped URL would
    // say "we broke" instead of "no such page".
    expect(roleAddressExists('backend', 'senior')).toBe(true);
    // A real address that is merely thin still EXISTS — the demand check is separate.
    expect(roleAddressExists('backend', 'junior')).toBe(true);
    expect(roleAddressExists('backend', 'archmage')).toBe(false);
    expect(roleAddressExists('not_a_category', 'senior')).toBe(false);
    // `other` is a real vocabulary value and not a role anybody hires for.
    expect(roleAddressExists('other', 'senior')).toBe(false);
  });

  it('refuses a thin role, an invented level, and an uncovered category', () => {
    const roles = [role('backend', 'senior', MIN_CATEGORY_OPEN)];

    expect(roleQualifies(roles, 'backend', 'senior', MIN_CATEGORY_OPEN)).toBe(true);
    expect(roleQualifies(roles, 'backend', 'junior', MIN_CATEGORY_OPEN - 1)).toBe(false);
    // An invented level is a wrong address, not a thin page.
    expect(roleQualifies(roles, 'backend', 'archmage', 5000)).toBe(false);
    expect(roleQualifies(roles, 'qa', 'senior', 5000)).toBe(false);
  });
});

describe('roleSkillsIntro', () => {
  it('says what the figures are measured over, never that they describe the market', () => {
    const intro = roleSkillsIntro({
      ...role('backend', 'senior', 4000),
      sample_size: 3000,
      skills: [
        { skill: 'docker', open_count: 2100, share: 0.7 },
        { skill: 'kubernetes', open_count: 1800, share: 0.6 },
        { skill: 'aws', open_count: 1500, share: 0.5 },
        { skill: 'go', open_count: 900, share: 0.3 },
      ],
    });

    // The sample, not the open count: only 39% of open technical postings state a
    // seniority, so the wider figure would claim a population this does not cover.
    expect(intro).toContain('3,000');
    expect(intro).not.toContain('4,000');
    expect(intro).toContain('docker, kubernetes, aws');
    expect(intro).not.toContain('go');
  });

  it('says so plainly when nothing cleared the floor', () => {
    const intro = roleSkillsIntro({ ...role('backend', 'senior', 40), sample_size: 0, skills: [] });

    expect(intro).toContain('Not enough');
  });
});
