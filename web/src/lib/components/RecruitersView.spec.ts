import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import RecruitersView from './RecruitersView.svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

describe('RecruitersView', () => {
  it('links employers and external job submitters to their respective flows', () => {
    render(RecruitersView);
    const postLinks = screen.getAllByRole('link', { name: 'Post a job for free' });
    expect(postLinks.map((link) => link.getAttribute('href'))).toEqual([
      '/my/employer/vacancies',
      '/my/employer/vacancies',
    ]);
    expect(screen.getByRole('link', { name: 'Submit an existing job' }).getAttribute('href')).toBe('/submit');
  });

  it('links hiring teams to the job promotion pilot', () => {
    render(RecruitersView);
    expect(screen.getByRole('link', { name: 'Explore the pilot' }).getAttribute('href')).toBe(
      '/recruiters/job-promotion',
    );
  });
});
