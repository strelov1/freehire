import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import JobMatchBar from './JobMatchBar.svelte';

const match = { percent: 62, matched: 5, total: 8 };

describe('JobMatchBar', () => {
  it('speaks the match in words, not through an ignored aria-label', () => {
    const { container } = render(JobMatchBar, { match });
    expect(screen.getByText('Profile match: 62%, 5 of 8 skills')).toBeTruthy();
    expect(container.querySelector('[aria-label]')).toBeNull();
  });

  it('says nothing under the teaser blur', () => {
    render(JobMatchBar, { match, blurred: true });
    expect(screen.queryByText(/Profile match/)).toBeNull();
  });
});
