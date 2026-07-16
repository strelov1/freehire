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
  categoryLabel,
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

describe('categoryLabel', () => {
  it('maps known tokens and title-cases unknowns', () => {
    expect(categoryLabel('ml_ai')).toBe('ML / AI');
    expect(categoryLabel('quantum_widgets')).toBe('Quantum Widgets');
  });
});
