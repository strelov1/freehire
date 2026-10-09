import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '$lib/types';
import AvoidCard from './AvoidCard.svelte';

const baseProfile: UserProfile = {
  specializations: ['backend'],
  skills: ['go', 'python'],
  seniorities: [],
  excluded_skills: ['java'],
  excluded_sources: ['greenhouse'],
  excluded_companies: ['acme'],
  location_preferences: null,
  derived_location: null,
  cv: null,
  created_at: null,
  updated_at: null,
};

const { avoidSkill, unavoidSkill, avoidSource, unavoidSource, avoidCompany, unavoidCompany, loadSkillDistribution } =
  vi.hoisted(() => ({
    avoidSkill: vi.fn(),
    unavoidSkill: vi.fn(),
    avoidSource: vi.fn(),
    unavoidSource: vi.fn(),
    avoidCompany: vi.fn(),
    unavoidCompany: vi.fn(),
    loadSkillDistribution: vi.fn(),
  }));

vi.mock('$app/state', () => ({ page: { data: {}, url: new URL('http://localhost/') } }));
vi.mock('$lib/profile.svelte', () => ({
  profileStore: {
    get profile() {
      return baseProfile;
    },
    avoidSkill,
    unavoidSkill,
    avoidSource,
    unavoidSource,
    avoidCompany,
    unavoidCompany,
  },
}));

// Both dictionary fetches are real network in production; stubbed here so the pickers'
// mount effects resolve instantly with no candidates to search.
vi.mock('$lib/skillDictionary', () => ({ loadSkillDistribution }));
vi.mock('$lib/sourceDictionary', () => ({
  loadSourceDistribution: vi.fn().mockResolvedValue([]),
}));
// Only companySearch is stubbed (real network in production); dynamicLabel is real so
// the fallback-label behavior (raw slug vs. resolved name) is exercised as written.
vi.mock('$lib/facets', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/facets')>()),
  companySearch: vi.fn().mockResolvedValue([]),
}));

beforeEach(() => {
  avoidSkill.mockReset().mockResolvedValue(baseProfile);
  unavoidSkill.mockReset().mockResolvedValue(baseProfile);
  avoidSource.mockReset().mockResolvedValue(baseProfile);
  unavoidSource.mockReset().mockResolvedValue(baseProfile);
  avoidCompany.mockReset().mockResolvedValue(baseProfile);
  unavoidCompany.mockReset().mockResolvedValue(baseProfile);
  loadSkillDistribution.mockReset().mockResolvedValue([]);
});

describe('AvoidCard', () => {
  it('notifies onProfileChanged after un-avoiding a skill succeeds', async () => {
    const onProfileChanged = vi.fn();
    render(AvoidCard, { props: { onProfileChanged } });

    await fireEvent.click(screen.getByTitle('java'));

    expect(unavoidSkill).toHaveBeenCalledWith('java');
    expect(onProfileChanged).toHaveBeenCalledTimes(1);
  });

  it('notifies onProfileChanged after un-avoiding a source succeeds', async () => {
    const onProfileChanged = vi.fn();
    render(AvoidCard, { props: { onProfileChanged } });

    // The chip's title is the resolved display name (sourceLabel), not the raw slug —
    // see the fallbackLabel fix: a pre-seeded value not in the dictionary's popular
    // page must still render its proper name, not "greenhouse".
    await fireEvent.click(screen.getByTitle('Greenhouse', { exact: true }));

    expect(unavoidSource).toHaveBeenCalledWith('greenhouse');
    expect(onProfileChanged).toHaveBeenCalledTimes(1);
  });

  it('notifies onProfileChanged after un-avoiding a company succeeds', async () => {
    const onProfileChanged = vi.fn();
    render(AvoidCard, { props: { onProfileChanged } });

    // Same resolved-label behavior as sources, via companyLabel.
    await fireEvent.click(screen.getByTitle('Acme', { exact: true }));

    expect(unavoidCompany).toHaveBeenCalledWith('acme');
    expect(onProfileChanged).toHaveBeenCalledTimes(1);
  });

  it('does not notify onProfileChanged when the save fails', async () => {
    unavoidSkill.mockReset().mockRejectedValue(new Error('network error'));
    const onProfileChanged = vi.fn();
    render(AvoidCard, { props: { onProfileChanged } });

    await fireEvent.click(screen.getByTitle('java'));

    expect(onProfileChanged).not.toHaveBeenCalled();
  });

  it('still un-avoids the skill when no onProfileChanged prop is given', async () => {
    render(AvoidCard, { props: {} });

    await fireEvent.click(screen.getByTitle('java'));

    expect(unavoidSkill).toHaveBeenCalledWith('java');
  });

  // "go" is one of the profile's currently-WANTED skills (baseProfile.skills). Offering
  // it as a candidate here would let a click silently un-claim it via
  // profileStore.avoidSkill (withAvoidedSkill drops a newly-avoided skill from `skills`)
  // with no warning on this tab — including, in the worst case, un-claiming a user's
  // only skill and turning the write into a 400 they'd see no explanation for.
  it('does not offer a currently-wanted skill as a candidate to avoid', async () => {
    loadSkillDistribution.mockResolvedValue([
      { value: 'go', label: 'Go' },
      { value: 'rust', label: 'Rust' },
    ]);
    render(AvoidCard, { props: {} });

    await fireEvent.focus(screen.getByPlaceholderText('Search skills to exclude'));
    await new Promise((resolve) => setTimeout(resolve, 300)); // clear the 250ms debounce

    // Matched on textContent, not accessible name: the techIcons SkillIcon renders an
    // <svg aria-label> alongside the visible text, which folds into the accessible name
    // and makes an exact-name match brittle.
    const optionTexts = screen.queryAllByRole('option').map((o) => o.textContent?.trim());
    expect(optionTexts.some((t) => t?.includes('Go'))).toBe(false);
    expect(optionTexts.some((t) => t?.includes('Rust'))).toBe(true);
  });
});
