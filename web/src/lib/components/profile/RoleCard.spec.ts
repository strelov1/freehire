import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { UserProfile } from '#lib/types.js';
import RoleCard from './RoleCard.svelte';

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' } } }));
vi.mock('#lib/profile.svelte.js', () => ({ profileStore: {} }));

const profile: UserProfile = {
  specializations: ['network_engineering', 'it_support'],
  skills: [],
  seniorities: [],
  excluded_skills: [],
  excluded_sources: [],
  excluded_companies: [],
  location_preferences: null,
  derived_location: null,
  cv: null,
  created_at: null,
  updated_at: null,
};

describe('profile specialization discovery', () => {
  it.each([
    ['SOC Analyst', 'Security'],
    ['Systems Administrator', 'DevOps'],
    ['Cloud Engineer', 'DevOps'],
    ['Helpdesk', 'IT Support / Help Desk'],
  ])('finds the specialization for %s', async (query, label) => {
    render(RoleCard, { profile });
    await fireEvent.input(screen.getByRole('textbox'), { target: { value: query } });
    expect(screen.getByRole('button', { name: label })).toBeTruthy();
  });

  it('links to jobs with all selected specializations', () => {
    render(RoleCard, { profile });
    const href = screen.getByRole('link', { name: 'Find jobs in these specializations' }).getAttribute('href');
    const url = new URL(href ?? '', 'https://freehire.me');
    expect(url.pathname).toBe('/jobs');
    expect(url.searchParams.get('category')).toBe('network_engineering,it_support');
    expect(url.searchParams.has('q')).toBe(false);
  });

  it('does not offer an unfiltered job link when no specialization is selected', () => {
    render(RoleCard, { profile: { ...profile, specializations: [] } });
    expect(screen.queryByRole('link', { name: 'Find jobs in these specializations' })).toBeNull();
  });
});
